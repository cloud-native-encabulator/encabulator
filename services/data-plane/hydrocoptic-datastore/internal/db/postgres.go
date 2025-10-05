package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/hydrocoptic-datastore/internal/models"
	_ "github.com/lib/pq"
)

type PostgresDB struct {
	db *sql.DB
}

func NewPostgresDB(connStr string) (*PostgresDB, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PostgresDB{db: db}, nil
}

func (p *PostgresDB) InitSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS encabulations (
		id VARCHAR(36) PRIMARY KEY,
		grammeters DOUBLE PRECISION NOT NULL,
		prefabulation BOOLEAN NOT NULL,
		sinusoidal_mode VARCHAR(50) NOT NULL,
		encabulation_factor DOUBLE PRECISION NOT NULL,
		dingle_arm_position DOUBLE PRECISION NOT NULL,
		prefabulation_rate DOUBLE PRECISION NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_encabulations_created_at ON encabulations(created_at);
	`
	_, err := p.db.Exec(schema)
	return err
}

func (p *PostgresDB) CreateEncabulation(req models.CreateEncabulationRequest) (*models.Encabulation, error) {
	enc := &models.Encabulation{
		ID:                 uuid.New().String(),
		Grammeters:         req.Grammeters,
		Prefabulation:      req.Prefabulation,
		SinusoidalMode:     req.SinusoidalMode,
		EncabulationFactor: req.Grammeters * 1.5,
		DingleArmPosition:  req.Grammeters * 0.75,
		PrefabulationRate:  calculatePrefabulationRate(req.Prefabulation, req.Grammeters),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	query := `
		INSERT INTO encabulations (id, grammeters, prefabulation, sinusoidal_mode, 
			encabulation_factor, dingle_arm_position, prefabulation_rate, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := p.db.Exec(query, enc.ID, enc.Grammeters, enc.Prefabulation, enc.SinusoidalMode,
		enc.EncabulationFactor, enc.DingleArmPosition, enc.PrefabulationRate, enc.CreatedAt, enc.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create encabulation: %w", err)
	}

	return enc, nil
}

func (p *PostgresDB) GetEncabulation(id string) (*models.Encabulation, error) {
	enc := &models.Encabulation{}
	query := `
		SELECT id, grammeters, prefabulation, sinusoidal_mode, encabulation_factor,
			dingle_arm_position, prefabulation_rate, created_at, updated_at
		FROM encabulations WHERE id = $1
	`
	err := p.db.QueryRow(query, id).Scan(
		&enc.ID, &enc.Grammeters, &enc.Prefabulation, &enc.SinusoidalMode,
		&enc.EncabulationFactor, &enc.DingleArmPosition, &enc.PrefabulationRate,
		&enc.CreatedAt, &enc.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get encabulation: %w", err)
	}

	return enc, nil
}

func (p *PostgresDB) ListEncabulations(page, pageSize int) ([]models.Encabulation, int, error) {
	offset := (page - 1) * pageSize

	// Get total count
	var total int
	err := p.db.QueryRow("SELECT COUNT(*) FROM encabulations").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count encabulations: %w", err)
	}

	// Get paginated results
	query := `
		SELECT id, grammeters, prefabulation, sinusoidal_mode, encabulation_factor,
			dingle_arm_position, prefabulation_rate, created_at, updated_at
		FROM encabulations
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := p.db.Query(query, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list encabulations: %w", err)
	}
	defer rows.Close()

	var encabulations []models.Encabulation
	for rows.Next() {
		var enc models.Encabulation
		err := rows.Scan(
			&enc.ID, &enc.Grammeters, &enc.Prefabulation, &enc.SinusoidalMode,
			&enc.EncabulationFactor, &enc.DingleArmPosition, &enc.PrefabulationRate,
			&enc.CreatedAt, &enc.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan encabulation: %w", err)
		}
		encabulations = append(encabulations, enc)
	}

	return encabulations, total, nil
}

func (p *PostgresDB) UpdateEncabulation(id string, req models.UpdateEncabulationRequest) (*models.Encabulation, error) {
	// First get the existing record
	enc, err := p.GetEncabulation(id)
	if err != nil {
		return nil, err
	}
	if enc == nil {
		return nil, nil
	}

	// Update fields if provided
	if req.Grammeters != nil {
		enc.Grammeters = *req.Grammeters
		enc.EncabulationFactor = *req.Grammeters * 1.5
		enc.DingleArmPosition = *req.Grammeters * 0.75
	}
	if req.Prefabulation != nil {
		enc.Prefabulation = *req.Prefabulation
	}
	if req.SinusoidalMode != nil {
		enc.SinusoidalMode = *req.SinusoidalMode
	}

	enc.PrefabulationRate = calculatePrefabulationRate(enc.Prefabulation, enc.Grammeters)
	enc.UpdatedAt = time.Now()

	query := `
		UPDATE encabulations 
		SET grammeters = $1, prefabulation = $2, sinusoidal_mode = $3,
			encabulation_factor = $4, dingle_arm_position = $5, prefabulation_rate = $6, updated_at = $7
		WHERE id = $8
	`
	_, err = p.db.Exec(query, enc.Grammeters, enc.Prefabulation, enc.SinusoidalMode,
		enc.EncabulationFactor, enc.DingleArmPosition, enc.PrefabulationRate, enc.UpdatedAt, id)

	if err != nil {
		return nil, fmt.Errorf("failed to update encabulation: %w", err)
	}

	return enc, nil
}

func (p *PostgresDB) DeleteEncabulation(id string) error {
	result, err := p.db.Exec("DELETE FROM encabulations WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete encabulation: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (p *PostgresDB) Close() error {
	return p.db.Close()
}

func (p *PostgresDB) Ping() error {
	return p.db.Ping()
}

func calculatePrefabulationRate(prefabulation bool, grammeters float64) float64 {
	if prefabulation {
		return grammeters * 2.5
	}
	return grammeters * 1.0
}
