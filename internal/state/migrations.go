package state

import "github.com/jacopofilonzi/wchron/internal/migrate"

// steps is the append-only list of state.json migrations: steps[i] turns a
// version i+1 document into version i+2 (see package migrate). Version 1 is
// the initial format, so the list is empty.
//
// To change the format: append a step and update the Go types, Default and
// Normalize. Never edit or reorder a step released on main. A step that
// touches existing data gets a test from a realistic previous-version file.
var steps = []migrate.Step{}

// CurrentVersion is the state.json format written by this app.
var CurrentVersion = migrate.Current(steps)
