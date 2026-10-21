// Package local manages downloaded assets and supervised child processes for
// parked optional engines. The default app does not link this package.
//
// Registry stores assets under <data>/models and third-party repos under
// <data>/third_party, with resumable downloads and progress callbacks.
// Process provides Start/Stop/Restart, crash backoff, and periodic health
// checks for optional child processes.
package local
