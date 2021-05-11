package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/mysql"
	"github.com/go-sql-driver/mysql"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_logrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	grpc_gateway "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/soheilhy/cmux"
	"go.opencensus.io/plugin/ocgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"
	sqltrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/database/sql"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/dynajoe/go-grpc-template/internal/config"
	dbsvc "github.com/dynajoe/go-grpc-template/internal/service/organization"
	pb "github.com/dynajoe/go-grpc-template/proto/go"
)

type Config struct {
	Env         string `envconfig:"ENV" default:"local"`
	ServiceName string `default:"organization_service"`

	DatabaseHost         string `envconfig:"DATABASE_HOST" validate:"required"`
	DatabasePort         string `envconfig:"DATABASE_PORT" default:"3306"`
	DatabaseUsername     string `envconfig:"DATABASE_USERNAME" validate:"required"`
	DatabasePassword     string `envconfig:"DATABASE_PASSWORD" validate:"required"`
	DatabaseName         string `envconfig:"DATABASE_NAME" validate:"required"`
	DatabaseMaxOpenConns int    `envconfig:"DATABASE_MAX_OPEN_CONNS" default:"10"`

	EnableTracing       bool   `default:"false"`
	DataDogApmAgentHost string `validate:"required"`
}

func main() {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})

	if err := start(logger); err != nil {
		logger.WithError(err).Fatal("failed to start")
	}
}

func start(logger *logrus.Logger) error {
	cfg := &Config{}
	if err := config.Load(cfg); err != nil {
		return err
	}
	if cfg.Env != "local" {
		cfg.EnableTracing = true
	}

	if cfg.EnableTracing {
		tracer.Start(
			tracer.WithAgentAddr(cfg.DataDogApmAgentHost),
			tracer.WithServiceName(cfg.ServiceName),
			tracer.WithEnv(cfg.Env),
		)
		defer tracer.Stop()
	}

	db, err := configureDB(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	dialect := goqu.Dialect("mysql")
	registerServices := func(s grpc.ServiceRegistrar) {
		pb.RegisterOrganizationServiceServer(s, dbsvc.NewService(dialect.DB(db)))
		grpc_health_v1.RegisterHealthServer(s, health.NewServer())
	}

	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		return err
	}
	defer ln.Close()

	// multiplex HTTP and gRPC on the same port
	lnmux := cmux.New(ln)
	lnhttp := lnmux.Match(cmux.HTTP1Fast())
	lngrpc := lnmux.Match(cmux.Any())

	// a run group is used to ensure all services
	// are running and shut down together
	g := &run.Group{}

	// basic kill signal handler
	stopCh := make(chan struct{})
	g.Add(func() error {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

		select {
		case <-sigCh:
		case <-stopCh:
		}

		return nil
	}, func(error) {
		close(stopCh)
	})

	// add the main net listener
	g.Add(func() error {
		return lnmux.Serve()
	}, func(error) {})

	// add the multiplexed listeners
	if err := addGRPCServe(logger, g, lngrpc, registerServices); err != nil {
		return err
	}

	if err := addHTTPServe(logger, g, lnhttp, ln.Addr().String()); err != nil {
		return err
	}

	return g.Run()
}

func addGRPCServe(logger *logrus.Logger, g *run.Group, ln net.Listener, registerServices func(grpc.ServiceRegistrar)) error {
	grpc_logrus.ReplaceGrpcLogger(logrus.NewEntry(logger))

	logOpts := []grpc_logrus.Option{
		grpc_logrus.WithDecider(func(methodFullName string, err error) bool {
			// always log errors
			if err != nil {
				return true
			}

			// do not log healthcheck
			if strings.HasPrefix(methodFullName, grpc_health_v1.Health_ServiceDesc.ServiceName) {
				return false
			}

			// log everything else
			return true
		}),
	}

	s := grpc.NewServer(
		grpc.StatsHandler(&ocgrpc.ServerHandler{}),
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(
			grpc_recovery.StreamServerInterceptor(),
			grpc_prometheus.StreamServerInterceptor,
		)),
		grpc_middleware.WithUnaryServerChain(
			grpc_logrus.UnaryServerInterceptor(logrus.NewEntry(logger), logOpts...),
			grpc_prometheus.UnaryServerInterceptor,
		),
	)

	registerServices(s)
	reflection.Register(s)
	grpc_prometheus.Register(s)

	stopCh := make(chan struct{})
	g.Add(func() error {
		go func() {
			<-stopCh
			s.GracefulStop()
		}()

		errCh := make(chan error)
		go func() {
			defer close(errCh)
			errCh <- s.Serve(ln)
		}()

		if err := <-errCh; err != nil {
			return err
		}
		return nil
	}, func(err error) {
		close(stopCh)
	})

	return nil
}

func addHTTPServe(logger *logrus.Logger, g *run.Group, ln net.Listener, grpcEndpoint string) error {
	ctx, cancel := context.WithCancel(context.Background())

	// gRPC dial options
	opts := []grpc.DialOption{
		grpc.WithInsecure(),
	}

	gwmux := grpc_gateway.NewServeMux(
		grpc_gateway.WithMarshalerOption(grpc_gateway.MIMEWildcard, &grpc_gateway.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				UseProtoNames: true,
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true,
			},
		}))

	err := pb.RegisterOrganizationServiceHandlerFromEndpoint(ctx, gwmux, grpcEndpoint, opts)
	if err != nil {
		cancel()
		return err
	}

	mainmux := http.NewServeMux()
	mainmux.Handle("/metrics", promhttp.Handler())
	mainmux.HandleFunc("/health", healthHandler)
	mainmux.Handle("/", gwmux)

	server := &http.Server{
		Addr:    grpcEndpoint,
		Handler: mainmux,
	}

	g.Add(func() error {
		// Shutdown when interrupted
		go func() {
			// Wait for signal to shutdown
			<-ctx.Done()

			// Attempt a graceful shutdown with timeout
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
			defer cancel()

			_ = server.Shutdown(ctx)
		}()

		if err := server.Serve(ln); err != http.ErrServerClosed {
			return err
		}

		return nil
	}, func(error) {
		cancel()
	})

	return nil
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("UP"))
}

func configureDB(cfg *Config) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		cfg.DatabaseUsername, cfg.DatabasePassword,
		cfg.DatabaseHost, cfg.DatabasePort,
		cfg.DatabaseName)

	var db *sql.DB
	var err error
	if cfg.EnableTracing {
		sqltrace.Register("mysql", &mysql.MySQLDriver{}, sqltrace.WithServiceName(cfg.ServiceName))
		db, err = sqltrace.Open("mysql", dsn)
	} else {
		db, err = sql.Open("mysql", dsn)
	}
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.DatabaseMaxOpenConns)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Minute * 5)

	return db, nil
}
