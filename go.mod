module github.com/businessperformancetuning/perfcollector

go 1.14

require (
	github.com/businessperformancetuning/license v0.0.0-20200905212554-7b6914fa9db8
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/dcrutil v1.4.0
	github.com/decred/slog v1.0.0
	github.com/inhies/go-bytesize v0.0.0-20200716184324-4fe85e9b81b2
	github.com/jessevdk/go-flags v1.4.0
	github.com/jmoiron/sqlx v1.2.0
	github.com/jrick/flagfile v0.0.0-20200516155228-eacdea149b40
	github.com/jrick/logrotate v1.0.0
	github.com/lib/pq v1.8.0
	golang.org/x/crypto v0.0.0-20200820211705-5c72a883971a
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sys v0.0.0-20200831180312-196b9ba8737a // indirect
)

replace github.com/businessperformancetuning/license => ../license
