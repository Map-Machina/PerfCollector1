package postgres

import (
	"context"
	"fmt"

	"github.com/businessperformancetuning/perfcollector/database"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type postgres struct {
	db   *sqlx.DB
	name string
}

var _ database.Database = (*postgres)(nil)

func (p *postgres) Open() error {
	log.Tracef("postgres.Open")

	if err := p.db.Ping(); err != nil {
		return err
	}

	// Verify database version
	var version int
	if err := p.db.Get(&version, database.SelectVersion); err != nil {
		log.Info("Creating database schema version 1")
		for k, v := range database.SchemaV1 {
			if _, err := p.db.Exec(v); err != nil {
				return fmt.Errorf("%v: %v", k, err)
			}
		}
		err = p.db.Get(&version, database.SelectVersion)
		if err != nil {
			return err
		}
	}

	// Run schema updates
	if version != database.Version {
		log.Infof("Upgrading database to version %v", database.Version)
		return fmt.Errorf("add database upgrade code here")
	}

	return nil
}

func (p *postgres) Close() error {
	log.Tracef("postgres.Close")

	return p.db.Close()
}

// Create creates the initial database.
func (p *postgres) Create() error {
	log.Tracef("postgres.Create")

	if err := p.Open(); err != nil {
		return err
	}
	defer p.Close()

	log.Infof("Creating database: %v", p.name)
	if _, err := p.db.Exec(fmt.Sprintf(database.CreateFormat, p.name)); err != nil {
		return err
	}
	log.Infof("Database version created: %v", database.Version)

	return nil
}

func (p *postgres) MeasurementsInsert(ctx context.Context, m *database.Measurements) (uint64, error) {
	log.Tracef("postgres.MeasurementsInsert")

	rows, err := p.db.NamedQueryContext(ctx, database.InsertMeasurements, m)
	if err != nil {
		return 0, err
	}
	var runId uint64
	if rows.Next() {
		err = rows.Scan(&runId)
		if err != nil {
			return 0, err
		}
	}
	return runId, nil
}

func (p *postgres) StatInsert(ctx context.Context, s []database.Stat) error {
	log.Tracef("postgres.StatInsert")

	// Use BeginTxx with ctx
	tx, err := p.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	for k := range s {
		_, err = tx.NamedExec(database.InsertStat, s[k])
		if err != nil {
			err2 := tx.Rollback()
			return fmt.Errorf("NamedExec: %v; Rollback: %v",
				err, err2)
		}
	}

	return tx.Commit()
}

func (p *postgres) MeminfoInsert(ctx context.Context, m *database.Meminfo) error {
	log.Tracef("postgres.MeminfoInsert")

	// Use BeginTxx with ctx
	tx, err := p.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	_, err = tx.NamedExec(database.InsertMeminfo, m)
	if err != nil {
		err2 := tx.Rollback()
		return fmt.Errorf("postgres.MeminfoInsert NamedExec: %v; "+
			"Rollback: %v", err, err2)
	}

	return tx.Commit()
}

func (p *postgres) NetDevInsert(ctx context.Context, nd []database.NetDev) error {
	log.Tracef("postgres.NetDevInsert")

	// Use BeginTxx with ctx
	tx, err := p.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	for k := range nd {
		_, err = tx.NamedExec(database.InsertNetDev, nd[k])
		if err != nil {
			err2 := tx.Rollback()
			return fmt.Errorf("postgres.NetDevInsert NamedExec: "+
				"%v; Rollback: %v", err, err2)
		}
	}

	return tx.Commit()
}

func (p *postgres) DiskstatInsert(ctx context.Context, ds []database.Diskstat) error {
	log.Tracef("postgres.DiskstatInsert")

	// Use BeginTxx with ctx
	tx, err := p.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	for k := range ds {
		_, err = tx.NamedExec(database.InsertDiskstat, ds[k])
		if err != nil {
			err2 := tx.Rollback()
			return fmt.Errorf("postgres.DiskstatInsert NamedExec: "+
				"%v; Rollback: %v", err, err2)
		}
	}

	return tx.Commit()
}

func (p *postgres) StatSelect(ctx context.Context, runID uint64) ([]database.Stat, error) {
	log.Tracef("postgres.StatSelect")

	var stats []database.Stat
	err := p.db.SelectContext(ctx, &stats, database.SelectStatByRunID, runID)
	if err != nil {
		return nil, fmt.Errorf("postgres.StatSelect: %w", err)
	}
	return stats, nil
}

func (p *postgres) MeminfoSelect(ctx context.Context, runID uint64) ([]database.Meminfo, error) {
	log.Tracef("postgres.MeminfoSelect")

	var meminfo []database.Meminfo
	err := p.db.SelectContext(ctx, &meminfo, database.SelectMeminfoByRunID, runID)
	if err != nil {
		return nil, fmt.Errorf("postgres.MeminfoSelect: %w", err)
	}
	return meminfo, nil
}

func (p *postgres) NetDevSelect(ctx context.Context, runID uint64) ([]database.NetDev, error) {
	log.Tracef("postgres.NetDevSelect")

	var netdev []database.NetDev
	err := p.db.SelectContext(ctx, &netdev, database.SelectNetDevByRunID, runID)
	if err != nil {
		return nil, fmt.Errorf("postgres.NetDevSelect: %w", err)
	}
	return netdev, nil
}

func (p *postgres) DiskstatSelect(ctx context.Context, runID uint64) ([]database.Diskstat, error) {
	log.Tracef("postgres.DiskstatSelect")

	var diskstat []database.Diskstat
	err := p.db.SelectContext(ctx, &diskstat, database.SelectDiskstatByRunID, runID)
	if err != nil {
		return nil, fmt.Errorf("postgres.DiskstatSelect: %w", err)
	}
	return diskstat, nil
}

func (p *postgres) MeasurementsSelect(ctx context.Context, runID uint64) (*database.Measurements, error) {
	log.Tracef("postgres.MeasurementsSelect")

	var m database.Measurements
	err := p.db.GetContext(ctx, &m, database.SelectMeasurementsByRunID, runID)
	if err != nil {
		return nil, fmt.Errorf("postgres.MeasurementsSelect: %w", err)
	}
	return &m, nil
}

func (p *postgres) ListRuns(ctx context.Context) ([]database.Measurements, error) {
	log.Tracef("postgres.ListRuns")

	var measurements []database.Measurements
	err := p.db.SelectContext(ctx, &measurements, database.SelectAllMeasurements)
	if err != nil {
		return nil, fmt.Errorf("postgres.ListRuns: %w", err)
	}
	return measurements, nil
}

func New(name, uri string) (*postgres, error) {
	log.Tracef("postgres.New")

	db, err := sqlx.Open("postgres", uri)
	if err != nil {
		return nil, err
	}
	return &postgres{db: db, name: name}, nil
}
