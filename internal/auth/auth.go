package auth

import (
	"fmt"
	"regexp"
)

type User struct {
	Name     string
	Password string
}

func ValidateUsername(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("there is no user name defined")
	}

	match, _ := regexp.MatchString("^[a-zA-Z]+[\\w]*$", name)

	if !match {
		return fmt.Errorf("Username can only contain letters, numbers or underscore")
	}

	return nil
}

func GetUser(name string, password string) (User, error) {
	if len(password) == 0 {
		return User{}, fmt.Errorf("there is no password defined")
	}

	err := ValidateUsername(name)

	if err != nil {
		return User{}, err
	}

	return User{Name: name, Password: password}, nil
}
