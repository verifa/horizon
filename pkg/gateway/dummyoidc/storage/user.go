package storage

import (
	"crypto/rsa"

	"golang.org/x/text/language"
)

type User struct {
	ID                string
	Username          string
	Groups            []string
	Password          string
	FirstName         string
	LastName          string
	Email             string
	EmailVerified     bool
	Phone             string
	PhoneVerified     bool
	PreferredLanguage language.Tag
	IsAdmin           bool
}

type Service struct {
	keys map[string]*rsa.PublicKey
}

type UserStore interface {
	GetUserByID(string) *User
	GetUserByUsername(string) *User
	ExampleClientID() string
}

type userStore struct {
	users map[string]*User
}

func NewUserStore(users map[string]*User) UserStore {
	return userStore{
		users: users,
		// users: map[string]*User{
		// 	"id1": {
		// 		ID:                "id1",
		// 		Username:          "test-user@" + hostname,
		// 		Groups:            []string{"group1", "group2"},
		// 		Password:          "verysecure",
		// 		FirstName:         "Test",
		// 		LastName:          "User",
		// 		Email:             "test-user@zitadel.ch",
		// 		EmailVerified:     true,
		// 		Phone:             "",
		// 		PhoneVerified:     false,
		// 		PreferredLanguage: language.German,
		// 		IsAdmin:           true,
		// 	},
		// 	"id2": {
		// 		ID:                "id2",
		// 		Username:          "test-user2",
		// 		Password:          "verysecure",
		// 		FirstName:         "Test",
		// 		LastName:          "User2",
		// 		Email:             "test-user2@zitadel.ch",
		// 		EmailVerified:     true,
		// 		Phone:             "",
		// 		PhoneVerified:     false,
		// 		PreferredLanguage: language.German,
		// 		IsAdmin:           false,
		// 	},
		// },
	}
}

// ExampleClientID is only used in the example server
func (u userStore) ExampleClientID() string {
	return "service"
}

func (u userStore) GetUserByID(id string) *User {
	return u.users[id]
}

func (u userStore) GetUserByUsername(username string) *User {
	for _, user := range u.users {
		if user.Username == username {
			return user
		}
	}
	return nil
}
