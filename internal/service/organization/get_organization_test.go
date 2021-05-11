package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	pb "github.com/dynajoe/go-grpc-template/proto/go"
)

type GetOrganizationTestSuite struct {
	suite.Suite
	svc *Service
}

func (s *GetOrganizationTestSuite) SetupTest() {
	s.svc = NewTestService(s.T())
}

func (s *GetOrganizationTestSuite) TestListDatabases_Success() {
	a := s.Assert()

	res, err := s.svc.GetOrganization(context.Background(), &pb.GetOrganizationRequest{
		OrganizationId: "TODO",
	})
	a.NoError(err)

	a.NotNil(res, "response should not be nil")
	a.NotNil(res.Organization, "organization should not be nil")
}

func TestGetOrganizationTestSuite(t *testing.T) {
	suite.Run(t, new(GetOrganizationTestSuite))
}
