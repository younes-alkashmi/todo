package auth

import "github.com/zalando/go-keyring"

const serviceName = "todo-app"

func StoreRefreshToken(uid, refreshToken string) error {
	return keyring.Set(serviceName, "refresh-"+uid, refreshToken)
}

func LoadRefreshToken(uid string) (string, error) {
	return keyring.Get(serviceName, "refresh-"+uid)
}

func DeleteRefreshToken(uid string) error {
	return keyring.Delete(serviceName, "refresh-"+uid)
}
