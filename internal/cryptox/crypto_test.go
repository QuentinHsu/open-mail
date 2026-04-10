package cryptox

import "testing"

func TestServiceEncryptDecryptRoundTrip(t *testing.T) {
	service := NewService("super-secret")

	ciphertext, err := service.Encrypt("hello-world")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	plaintext, err := service.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if plaintext != "hello-world" {
		t.Fatalf("Decrypt() = %q, want %q", plaintext, "hello-world")
	}
}
