package soroauth

import (
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// BenchmarkDecodeAuthorizationEntry measures the bounded decode of an entry
// that came from somewhere else, which is the path Inspect and the CLI use for
// untrusted input (issue #121).
//
// The three signing-path benchmarks this file once carried (Preimage, Payload
// and AuthorizeEntry) live in bench_signing_test.go, which came from issue #107
// and is the copy that testdata/bench/budgets.json gates; duplicating the names
// here only made the package fail to build.
//
// The entry is built with benchAddressEntry so its bytes have the same shape as
// the signing-path benchmarks' input. ns/op is machine-dependent, and this
// benchmark has no budget entry, so checkbench reports it as a warning rather
// than gating on it.
func BenchmarkDecodeAuthorizationEntry(b *testing.B) {
	entry := benchAddressEntry(b, xdr.SorobanCredentialsTypeSorobanCredentialsAddressV2,
		"soroauth-bench-signer", 42)
	encoded, err := xdr.MarshalBase64(entry)
	if err != nil {
		b.Fatalf("encoding the benchmark entry: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := DecodeAuthorizationEntry(encoded); err != nil {
			b.Fatalf("DecodeAuthorizationEntry returned an unexpected error: %v", err)
		}
	}
}
