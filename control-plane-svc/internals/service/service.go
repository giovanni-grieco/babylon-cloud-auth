package service

import (
	"fmt"
	"log"
)

type ControlPlaneService struct {
}

func NewControlPlanceService() *ControlPlaneService {
	return new(ControlPlaneService)
}

func (cps *ControlPlaneService) DiscoverRealm(userHint string) (string, error) {
	// TODO: Implement realm discovery logic
	if userHint == "alice" || userHint == "alice@dc1.test" {
		return "dc-1", nil
	}

	if userHint == "bob" || userHint == "bob@dc2.test" {
		return "dc-2", nil
	}

	return "", fmt.Errorf("realm not found for user hint: %s", userHint)
}

func (cps *ControlPlaneService) RegisterUser(username, email, temporaryPassword string) error {
	// TODO: Implement user registration logic
	log.Printf("registering user: %s - %s - %s", username, email, temporaryPassword)
	return nil
}
