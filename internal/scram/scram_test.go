package scram

import "testing"

func TestRFC7677Vector(t *testing.T) {
	client, err := NewWithNonce("user", "pencil", "rOprNGfwEbeRWgbNEkqO")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := client.FirstMessage(), "n,,n=user,r=rOprNGfwEbeRWgbNEkqO"; got != want {
		t.Fatalf("first message = %q, want %q", got, want)
	}
	serverFirst := "r=rOprNGfwEbeRWgbNEkqO%hvYDpWUa2RaTCAfuxFIlj)hNlF$k0,s=W22ZaJ0SNY7soEsUEjb6gQ==,i=4096"
	final, err := client.Continue(serverFirst)
	if err != nil {
		t.Fatal(err)
	}
	wantFinal := "c=biws,r=rOprNGfwEbeRWgbNEkqO%hvYDpWUa2RaTCAfuxFIlj)hNlF$k0,p=dHzbZapWIk4jUhN+Ute9ytag9zjfMHgsqmmiz7AndVQ="
	if final != wantFinal {
		t.Fatalf("final message = %q, want %q", final, wantFinal)
	}
	if err := client.Final("v=6rriTRBi23WpRR/wtup+mMhUZUn/dB5nLTJRsjl95G4="); err != nil {
		t.Fatal(err)
	}
}

func TestSCRAMRejectsNonceDowngradeAndSignatureMismatch(t *testing.T) {
	client, err := NewWithNonce("user", "password", "clientnonce")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Continue("r=other,s=W22ZaJ0SNY7soEsUEjb6gQ==,i=4096"); err == nil {
		t.Fatal("expected nonce error")
	}
	client, _ = NewWithNonce("user", "password", "clientnonce")
	_, err = client.Continue("r=clientnonce-server,s=W22ZaJ0SNY7soEsUEjb6gQ==,i=4096")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Final("v=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="); err == nil {
		t.Fatal("expected signature mismatch")
	}
}
