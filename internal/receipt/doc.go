// Package receipt manages per-tool installation receipts stored alongside
// binaries in bin_dir. A receipt records the installed version and the
// SHA-256 of the binary at install time.
//
// Before a tool is trusted as already-installed, [Verify] performs a
// two-step check: the receipt's version must match the desired version,
// and the binary on disk must hash to the checksum recorded at install
// time. This catches corruption, manual replacement, and tampering
// independently of the upstream checksum verification performed during
// download.
//
// Receipt files are named .{tool}.receipt and written atomically
// alongside the binary in bin_dir.
package receipt
