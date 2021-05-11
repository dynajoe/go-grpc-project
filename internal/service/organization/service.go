package database

import (
	"github.com/doug-martin/goqu/v9"

	pb "github.com/dynajoe/go-grpc-template/proto/go"
)

type Service struct {
	pb.UnimplementedOrganizationServiceServer

	db *goqu.Database
}

func NewService(db *goqu.Database) *Service {
	return &Service{db: db}
}

var (
	_ pb.OrganizationServiceServer = &Service{}
)
