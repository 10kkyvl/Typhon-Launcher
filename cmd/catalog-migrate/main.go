package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"typhon/internal/catalogmigration"
)

func main() {
	dir := flag.String("dir", "", "explicit offline data directory")
	apply := flag.String("apply", "", "apply a previously saved JSON report; app must be stopped")
	restore := flag.String("restore", "", "restore a redirect backup directory; app must be stopped")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "--dir is required; no default user data location")
		os.Exit(2)
	}
	var err error
	if *restore != "" {
		err = catalogmigration.Restore(*dir, *restore)
	} else if *apply != "" {
		var report catalogmigration.Report
		var b []byte
		b, err = os.ReadFile(*apply)
		if err == nil {
			err = json.Unmarshal(b, &report)
		}
		if err == nil {
			err = catalogmigration.Apply(*dir, report)
		}
	} else {
		var report catalogmigration.Report
		report, err = catalogmigration.Plan(*dir)
		if err == nil {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			err = enc.Encode(report)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
