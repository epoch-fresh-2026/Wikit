package security

import (
	"strings"
	"testing"
)

func TestPasswordAndEntropy(t *testing.T) {
	p, err := NewPasswords()
	if err != nil {
		t.Fatal(err)
	}
	one, err := p.Hash("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	two, err := p.Hash("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if one == two || !strings.HasPrefix(one, "$argon2id$v=19$") {
		t.Fatal("missing independent salt or Argon2id")
	}
	if !p.Verify("correct-password", one) || p.Verify("incorrect-password", one) || p.Verify("anything", "") || p.Verify("anything", "$argon2id$v=19$m=999999999,t=3,p=4$x$x") {
		t.Fatal("password verification failed")
	}
	s := Secrets{}
	a, err := s.Secret()
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Secret()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 43 || a == b {
		t.Fatal("bad entropy")
	}
}
func BenchmarkArgon2id(b *testing.B) {
	p, err := NewPasswords()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.Hash("benchmark-password"); err != nil {
			b.Fatal(err)
		}
	}
}
