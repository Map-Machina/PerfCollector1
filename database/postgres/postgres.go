package postgres

import (
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

func (p *postgres) MeasurementsInsert(m *database.Measurements) (uint64, error) {
	log.Tracef("postgres.MeasurementsInsert")

	rows, err := p.db.NamedQuery(database.InsertMeasurements, m)
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

func (p *postgres) MeminfoInsert(mi *database.Meminfo2) error {
	log.Tracef("postgres.MeminfoInsert")

	_, err := p.db.NamedExec(database.InsertMeminfo2, mi)
	return err
}

func New(name, uri string) (*postgres, error) {
	log.Tracef("postgres.New")

	db, err := sqlx.Open("postgres", uri)
	if err != nil {
		return nil, err
	}
	return &postgres{db: db, name: name}, nil
}
