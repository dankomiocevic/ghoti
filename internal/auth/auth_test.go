package auth

import (
	"testing"
)

func TestCreateUser(t *testing.T) {
	user, e := GetUser("name", "pass")

	if e != nil {
		t.Fatalf("user creation returned an error: %s", e)
	}

	if user.Name != "name" {
		t.Fatalf("user name must be name")
	}

	if user.Password != "pass" {
		t.Fatalf("user password must be pass")
	}
}

func TestEmptyPassword(t *testing.T) {
	_, e := GetUser("name", "")

	if e == nil {
		t.Fatalf("user creation did not return an error")
	}
}

func TestEmptyUser(t *testing.T) {
	_, e := GetUser("", "pass")

	if e == nil {
		t.Fatalf("user creation did not return an error")
	}
}

func TestUsernameSpecialChars(t *testing.T) {
	_, e := GetUser("?user", "pass")

	if e == nil {
		t.Fatalf("user creation did not return an error")
	}
}

func TestUsernameStartsWithNumber(t *testing.T) {
	_, e := GetUser("2name3", "pass")

	if e == nil {
		t.Fatalf("user creation did not return an error")
	}
}

func TestUsernameWithLettersAndNumbers(t *testing.T) {
	user, e := GetUser("name3_2abc", "pass")

	if e != nil {
		t.Fatalf("user creation returned an error: %s", e)
	}

	if user.Name != "name3_2abc" {
		t.Fatalf("user name must be name3_2abc")
	}
}
