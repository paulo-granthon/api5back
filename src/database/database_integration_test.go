//go:build integration
// +build integration

package database

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"api5back/ent"
	"api5back/ent/migrate"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startTestingDatabaseContainer(
	ctx context.Context,
	credentials *Credentials,
) (testcontainers.Container, error) {
	var databaseName string
	if credentials.Name != nil {
		databaseName = fmt.Sprintf("%s", *credentials.Name)
	} else {
		databaseName = ""
	}

	req := testcontainers.ContainerRequest{
		Image:        "postgres:latest",
		Name:         "khali-api5-TI-postgres",
		ExposedPorts: []string{"5432/tcp"},
		HostConfigModifier: func(hc *container.HostConfig) {
			hc.PortBindings = nat.PortMap{
				"5432/tcp": []nat.PortBinding{{
					HostIP:   "localhost",
					HostPort: fmt.Sprintf("%d/tcp", *credentials.Port),
				}},
			}
		},
		// Wait for _this string_ to appear in the container logs
		// it's the last unique string that appears, but not the last line.
		// The last line appears the first time way sooner so wouldn't work.
		// I've also tried this, no bueno:
		//		WaitingFor: wait.ForListeningPort("5432/tcp"),
		WaitingFor: wait.ForLog("listening on IPv6 address"),
		Env: map[string]string{
			"POSTGRES_USER":     credentials.User,
			"POSTGRES_PASSWORD": credentials.Pass,
			"POSTGRES_DB":       databaseName,
		},
	}

	// Start the container
	return testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		},
	)
}

type TestCase struct {
	Name string
	Run  func(t *testing.T)
}

func TestDatabaseOperations(t *testing.T) {
	ctx := context.Background()
	var client *ent.Client
	var err error

	if testResult := t.Run("Setup database connection", func(t *testing.T) {
		credentials, err := newTestingCredentials()
		require.NoError(t, err)

		if _, err = startTestingDatabaseContainer(
			ctx,
			credentials,
		); err != nil {
			t.Fatalf("failed to start the testing database container: %v", err)
		}

		databaseUrl := credentials.getConnectionString()

		client, err = createPostgresClient(databaseUrl)
		require.NoError(t, err)
	}); !testResult {
		t.Fatalf("Setup test failed")
	}

	defer client.Close()

	// não acredito q tem q botar um sleep aqui ¬¬
	// Sem o sleep ainda fica faltando esse mei segundo pro container
	// aceitar a conexão após o `listening on IPv6 address`...
	// na minha máquina foi uns 200ms (dá pra consultar nos logs)
	time.Sleep(time.Duration(
		getContainerConnectionDelayMs(),
	) * time.Millisecond)

	if testResult := t.Run("Migrate database", func(t *testing.T) {
		if err = client.Schema.Create(
			ctx,
			migrate.WithDropIndex(true),
			migrate.WithDropColumn(true),
		); err != nil {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("failed to migrate the database: %v", err))
			sb.WriteString("\n\nThis error may be caused by the test not waiting long enough for the database to be ready.")
			sb.WriteString("\nTry increasing the sleep time in the `.env.integration` test.")
			t.Fatalf(sb.String())
		}
	}); !testResult {
		t.Fatalf("Migration test failed")
	}

	t.Run("Test dim_user table operations", func(t *testing.T) {
		var testDimUser *ent.DimUser

		for _, TestCase := range []TestCase{
			{
				Name: "Insert a dim_user into the table",
				Run: func(t *testing.T) {
					testDimUser, err = client.DimUser.
						Create().
						SetName("John Doe").
						SetOcupation("Software Engineer").
						Save(ctx)
					if err != nil {
						t.Fatalf("failed to insert the dim_user: %v", err)
					}
					require.Equal(t, "John Doe", testDimUser.Name)
					require.Equal(t, "Software Engineer", testDimUser.Ocupation)
				},
			}, {
				Name: "Retrieve the inserted dim_user",
				Run: func(t *testing.T) {
					retrievedDimUser, err := client.DimUser.Get(ctx, testDimUser.ID)
					if err != nil {
						t.Fatalf("failed to retrieve the dim_user: %v", err)
					}
					require.Equal(t, testDimUser.ID, retrievedDimUser.ID)
					require.Equal(t, testDimUser.Name, retrievedDimUser.Name)
					require.Equal(t, testDimUser.Ocupation, retrievedDimUser.Ocupation)
				},
			}, {
				Name: "Delete the dim_user",
				Run: func(t *testing.T) {
					err = client.DimUser.DeleteOne(testDimUser).Exec(ctx)
					require.NoError(t, err)
				},
			}, {
				Name: "Try to retrieve the dim_user again, expecting a not found error",
				Run: func(t *testing.T) {
					_, err = client.DimUser.Get(ctx, testDimUser.ID)
					require.Error(t, err)
				},
			},
		} {
			if testResult := t.Run(TestCase.Name, TestCase.Run); !testResult {
				t.Fatalf("Test case failed")
			}
		}
	})
}

func getContainerConnectionDelayMs() int {
	containerConnectionDelayMs := 500

	if err := godotenv.Load("../../.env.integration"); err != nil {
		return containerConnectionDelayMs
	}

	containerConnectionDelayMsStr, ok := os.LookupEnv("CONTAINER_CONNECTION_DELAY_MS")
	if !ok || containerConnectionDelayMsStr == "" {
		return containerConnectionDelayMs
	}

	containerConnectionDelayMs, err := strconv.Atoi(containerConnectionDelayMsStr)
	if err != nil {
		return containerConnectionDelayMs
	}

	if containerConnectionDelayMs < 0 {
		return 0
	}

	return containerConnectionDelayMs
}
