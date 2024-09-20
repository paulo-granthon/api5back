package database

import (
	"api5back/ent"
	"context"
	"database/sql"
	"fmt"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func DatabaseSetup(prefix string) (*ent.Client, error) {
	databaseCredentials, err := NewDatabaseCredentials(prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to create databaseCredentials: %v", err)
	}
	databaseUrl := databaseCredentials.GetConnectionString()

	client, err := CreatePostgresClient(databaseUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres client: %v", err)
	}

	if err := Migrate(client); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %v", err)
	}

	return client, nil
}

func CreatePostgresClient(databaseUrl string) (*ent.Client, error) {
	db, err := sql.Open("pgx", databaseUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	drv := entsql.OpenDB(dialect.Postgres, db)
	return ent.NewClient(ent.Driver(drv)), nil
}

// Run the automatic migration tool to create all schema resources.
func Migrate(client *ent.Client) error {
	ctx := context.Background()

	if err := client.Schema.Create(ctx); err != nil {
		return fmt.Errorf("failed creating schema resources: %v", err)
	}

	return nil
}
