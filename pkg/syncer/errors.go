package syncer

import "fmt"

type MissingEnvironmentVariablesError struct {
	variables []string
}

func (e *MissingEnvironmentVariablesError) Error() string {
	message := "the following environment variables are missing or empty: "
	for i := range e.variables {
		if lastIndex := len(e.variables) - 1; i != lastIndex {
			message += fmt.Sprintf("%s, ", e.variables[i])
			continue
		}
		message += e.variables[i]
	}
	return message
}
