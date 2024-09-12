package auth

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
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

func Login(value string) (User, error) {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return User{}, fmt.Errorf("User cannot be base64 decoded")
	}

	userArray := strings.SplitN(string(data), ":", 2)
	if len(userArray) != 2 {
		return User{}, fmt.Errorf("Missing username or password")
	}

	user, err := GetUser(userArray[0], userArray[1])
	if err != nil {
		return User{}, err
	}

	return user, nil
}
