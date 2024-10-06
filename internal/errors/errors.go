package errors

import (
	_ "embed"
	"fmt"
	"regexp"
	"sync"
)

type ErrorCode struct {
	name string
	id   string
}

//go:embed README.md
var readme string
var values map[string]ErrorCode
var once sync.Once

func Error(name string) ErrorCode {
	once.Do(func() {
		values = loadValues()
	})

	return values[name]
}

func (e ErrorCode) String() string {
	return e.id
}

func loadValues() map[string]ErrorCode {
	r := regexp.MustCompile(`## \d\d\d: [A-Z_]*`)
	matches := r.FindAllString(readme, -1)

	m := make(map[string]ErrorCode)
	for _, v := range matches {
		id := fmt.Sprint(v[3:6])
		name := fmt.Sprint(v[8:len(v)])

		e := ErrorCode{name: name, id: id}
		m[name] = e
	}

	return m
}
