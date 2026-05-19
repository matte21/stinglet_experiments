package trace

import (
	"fmt"
	"os"

	"database/sql"

	_ "modernc.org/sqlite"
)

// Invoke cleanup only if err == nil (you can invoke it even if err != nil, but it's a nop).
func Get(dbFilePath string, machineID int, cxlPercentageColumn string) (trace *sql.Rows, cleanup func(), err error) {
	cleanup = func() {}

	db, err := sql.Open("sqlite", dbFilePath)
	if err != nil {
		err = fmt.Errorf("failed to open trace DB %s: %w", dbFilePath, err)
		return
	}
	defer func() {
		if err != nil {
			if err1 := db.Close(); err1 != nil {
				fmt.Fprintf(os.Stderr, "Failed to close trace DB %s: %v\n", dbFilePath, err1)
			}
		}
	}()

	var query string
	if cxlPercentageColumn == "nop" {
		query = fmt.Sprintf(`SELECT vmId, priority, cores, memory FROM "events-machine-id-%d" ORDER BY time`, machineID)
	} else {
		query = fmt.Sprintf(`SELECT vmId, priority, cores, memory, cxl_mem_percent_%s FROM "events-machine-id-%d" ORDER BY time`,
			cxlPercentageColumn, machineID)
	}
	trace, err = db.Query(query)
	if err != nil {
		err = fmt.Errorf("failed to run query \"%s\" against trace DB %s: %w", query, dbFilePath, err)
		return
	}

	cleanup = func() {
		if err := trace.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close trace DB %s rows: %v\n", dbFilePath, err)
		}
		if err := db.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close trace DB %s: %v\n", dbFilePath, err)
		}
	}

	return
}
