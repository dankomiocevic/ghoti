package errors

type TranscientError struct {
	Err string
}

func (e TranscientError) Error() string {
	return e.Err
}

func (e TranscientError) String() string {
	return e.Err
}

type PermanentError struct {
	Err string
}

func (e PermanentError) Error() string {
	return e.Err
}

func (e PermanentError) String() string {
	return e.Err
}
