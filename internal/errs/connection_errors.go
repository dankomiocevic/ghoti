package errs

type TranscientError struct {
	Err string
}

func (e TranscientError) Error() string {
	return e.Err
}

type PermanentError struct {
	Err string
}

func (e PermanentError) Error() string {
	return e.Err
}
