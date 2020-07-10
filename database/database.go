package database

import "github.com/businessperformancetuning/perfcollector/parser"

type Database interface {
	Create() error // Create schema. Database is NOT Opened!
	Open() error   // Open database connection and create+upgrade schema
	Close() error  // Close database

	MeminfoInsert(*parser.Meminfo) error // Insert Meminfo into the database
}

const (
	Name    = "performancedata"
	Version = 1
)

var (
	Create        = "CREATE DATABASE " + Name + ";"
	SelectVersion = "SELECT * FROM version LIMIT 1;"
	MeminfoInsert = "INSERT INTO users (Id, Email, Password, Admin) VALUES (:id, :email, :password, :admin);"
)

var (
	SchemaV1 = []string{`
CREATE TABLE version (Version int);
`, `
INSERT INTO version (Version) VALUES (1);
`, `
CREATE TABLE meminfo (
	mem_total		BIGINT,
	mem_free		BIGINT,
	mem_available		BIGINT,
	buffers			BIGINT,
	cached			BIGINT,
	swap_cached		BIGINT,
	active			BIGINT,
	inactive		BIGINT,
	active_anon		BIGINT,
	inactive_anon		BIGINT,
	active_file		BIGINT,
	inactive_file		BIGINT,
	unevictable		BIGINT,
	mlocked			BIGINT,
	swap_total		BIGINT,
	swap_free		BIGINT,
	dirty			BIGINT,
	write_back		BIGINT,
	anon_pages		BIGINT,
	mapped			BIGINT,
	shmem			BIGINT,
	slab			BIGINT,
	sreclaimable		BIGINT,
	sunreclaim		BIGINT,
	Kernel_stack		BIGINT,
	page_tables		BIGINT,
	nfs_unstable		BIGINT,
	bounce			BIGINT,
	write_back_tmp		BIGINT,
	commit_limit		BIGINT,
	commit_as		BIGINT,
	vmalloc_total		BIGINT,
	vmalloc_used		BIGINT,
	vmalloc_chunk		BIGINT,
	hardware_corrupted	BIGINT,
	anon_huge_pages		BIGINT,
	shmem_huge_pages	BIGINT,
	shmem_pmd_mapped	BIGINT,
	cma_total		BIGINT,
	cma_free		BIGINT,
	huge_pages_total	BIGINT,
	huge_pages_free		BIGINT,
	huge_pages_rsvd		BIGINT,
	huge_pages_surp		BIGINT,
	huge_pages_size		BIGINT,
	direct_map_4k		BIGINT,
	direct_map_2m		BIGINT,
	direct_map_1g		BIGINT);
`}
)
