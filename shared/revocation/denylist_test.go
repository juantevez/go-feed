package revocation

import (
	"testing"
	"time"
)

func TestOnRevokeNotifiesNewRevocationsOnce(t *testing.T) {
	d := &Denylist{revoked: make(map[string]time.Time)}
	var got []string
	d.OnRevoke(func(jti string) { got = append(got, jti) })

	exp := time.Now().Add(time.Minute)
	d.set("a", exp)
	d.set("a", exp)                                // Revoke + watcher traen el mismo jti
	d.set("expired", time.Now().Add(-time.Second)) // ya vencido: no hay nada que cortar
	d.set("b", exp)

	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("notified = %v, want [a b]", got)
	}
	if !d.IsRevoked("a") || !d.IsRevoked("b") {
		t.Error("revoked tokens must still be reported by IsRevoked")
	}
}
