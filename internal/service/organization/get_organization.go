package database

import (
	"context"

	"github.com/doug-martin/goqu/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/dynajoe/go-grpc-template/internal/model"
	pb "github.com/dynajoe/go-grpc-template/proto/go"
)

func (s *Service) GetOrganization(ctx context.Context, req *pb.GetOrganizationRequest) (*pb.GetOrganizationResponse, error) {
	org, err := getOrganization(ctx, s.db, req.OrganizationId)
	if err != nil {
		return nil, err
	}

	orgProto, err := model.OrganizationToProto(org)
	if err != nil {
		return nil, err
	}

	return &pb.GetOrganizationResponse{
		Organization: orgProto,
	}, nil
}

func getOrganization(ctx context.Context, db *goqu.Database, organizationID string) (*model.Organization, error) {
	sql := `
		SELECT o.organization_uuid, o.name
		FROM organization o
		WHERE o.organization_uuid = ?;
	`

	var org model.Organization
	if found, err := db.ScanStructContext(ctx, &org, sql, organizationID); err != nil {
		return nil, status.Errorf(codes.Unavailable, "error fetching organization: %v", err)
	} else if !found {
		return nil, status.Errorf(codes.NotFound, "organization not found")
	}

	return &org, nil
}
