package auth

import "testing"

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"+7 (900) 000-00-01", "+79000000001", true},
		{"8 900 000 00 01", "+79000000001", true},
		{"9000000001", "+79000000001", true},
		{"+1 202 555 0147", "+12025550147", true},
		{"123", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := NormalizePhone(tt.in)
		if ok != tt.ok || got != tt.want {
			t.Errorf("NormalizePhone(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	if _, ok := NormalizeEmail("Client@Example.RU"); !ok {
		t.Fatal("valid email rejected")
	}
	if _, ok := NormalizeEmail("not-an-email"); ok {
		t.Fatal("invalid email accepted")
	}
}

func TestPasswordHash(t *testing.T) {
	hash, err := HashPassword("Secret123!")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "Secret123!" {
		t.Fatal("password must be hashed")
	}
	if !CheckPassword(hash, "Secret123!") {
		t.Fatal("password check failed")
	}
	if CheckPassword(hash, "Wrong123!") {
		t.Fatal("wrong password accepted")
	}
	if _, err := HashPassword("123"); err == nil {
		t.Fatal("short password accepted")
	}
}

func TestGeneratePassword(t *testing.T) {
	p, err := GeneratePassword()
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(p)) != 12 {
		t.Fatalf("unexpected password length: %q", p)
	}
	hash, err := HashPassword(p)
	if err != nil || !CheckPassword(hash, p) {
		t.Fatal("generated password does not hash/check")
	}
}
