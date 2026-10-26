package config

import "github.com/jacopofilonzi/wchron/internal/migrate"

// steps is the append-only list of config.json migrations: steps[i] turns a
// version i+1 document into version i+2 (see package migrate). Version 1 is
// the initial format, so the list is empty.
//
// To change the format: append a step (a func on the raw JSON object), which
// bumps CurrentVersion, then update the Go types, Default and Normalize. Never
// edit or reorder a step released on main. A step that touches existing data
// gets a test that loads a realistic file at the previous version.
var steps = []migrate.Step{}

// CurrentVersion is the config.json format written by this app.
var CurrentVersion = migrate.Current(steps)
