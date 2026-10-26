package platform

// Opening the source files of a run. Excel (and other programs) keep the
// files they have open with their own sharing mode: the app must open them
// asking only for read access and allowing everyone else to read, write and
// delete, so it never blocks the other program and is refused only when that
// program really holds the file exclusively (a lock, IsLocked).
//
// OpenShared is the only way the resolver and the processors open an
// original file. OpenExclusive exists for the tests: it simulates a program
// holding a file exclusively (on Windows; elsewhere it is unsupported and
// the tests skip).
