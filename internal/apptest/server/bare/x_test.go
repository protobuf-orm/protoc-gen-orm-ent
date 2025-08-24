package bare_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"

	"entgo.io/ent/dialect/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type Server struct {
	t *testing.T

	Db *ent.Client
	apptest.Server
}

func NewServer(t *testing.T) *Server {
	ctx := context.TODO()

	driver, err := sql.Open("sqlite3", ":memory:?cache=shared&_fk=1")
	require.NoError(t, err)

	driver.DB().SetMaxOpenConns(1)

	db := ent.NewClient(ent.Driver(driver))
	db = db.Debug()
	err = db.Schema.Create(ctx)
	require.NoError(t, err)

	s := bare.NewServer(db)
	return &Server{
		t: t,

		Db:     ent.NewClient(ent.Driver(driver)),
		Server: s,
	}
}

func (s *Server) Grpc() *grpc.Server {
	opts := []grpc.ServerOption{
		grpc.Creds(insecure.NewCredentials()),
	}

	v := grpc.NewServer(opts...)
	apptest.RegisterServer(v, s)
	return v
}

func (s *Server) Close() error {
	if err := s.Db.Close(); err != nil {
		return fmt.Errorf("close DB: %w", err)
	}

	return nil
}

type Client struct {
	T *testing.T

	Server *Server
	apptest.Client

	Listener *bufconn.Listener
	Conn     grpc.ClientConnInterface

	wg sync.WaitGroup

	grpc_server *grpc.Server
}

func NewClient(t *testing.T, s *Server) *Client {
	listener := bufconn.Listen(1 << 20)
	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
	)
	require.NoError(t, err)

	v := &Client{
		T: t,

		Server:   s,
		Listener: listener,
		Conn:     conn,

		grpc_server: s.Grpc(),
		Client:      apptest.NewClient(conn),
	}

	v.wg.Add(1)
	go func() {
		defer v.wg.Done()
		if err := v.grpc_server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			require.Fail(t, fmt.Sprintf("server failed: %s", err.Error()))
		}
	}()

	return v
}

func (c *Client) Close() error {
	c.grpc_server.GracefulStop()
	c.wg.Wait()
	return nil
}

func T(run func(ctx context.Context, x *require.Assertions, c *Client)) func(t *testing.T) {
	return func(t *testing.T) {
		s := NewServer(t)
		defer s.Close()

		c := NewClient(t, s)
		defer c.Close()

		x := require.New(t)
		run(t.Context(), x, c)
	}
}
