package auth

import (
	"fmt"
	"regexp"
)

type User struct {
	Name     string
	Password string
}

func GetUser(name string, password string) (User, error) {
	if len(name) == 0 {
		return User{}, fmt.Errorf("there is no user name defined")
	}

	if len(password) == 0 {
		return User{}, fmt.Errorf("there is no password defined")
	}

	match, _ := regexp.MatchString("^[a-zA-Z]+[\\w]*$", name)

	if !match {
		return User{}, fmt.Errorf("Username can only contain letters, numbers or underscore")
	}

	return User{Name: name, Password: password}, nil
}
