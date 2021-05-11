package model

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/dynajoe/go-grpc-template/proto/go"
)

type Organization struct {
	OrganizationID string `db:"organization_uuid"`
	Name           string `db:"name"`
}

func OrganizationToProto(db *Organization) (*pb.Organization, error) {
	if db == nil {
		return nil, status.Error(codes.Internal, "cannot convert nil to proto")
	}

	return &pb.Organization{
		OrganizationId: db.OrganizationID,
		Name:           db.Name,
	}, nil
}
