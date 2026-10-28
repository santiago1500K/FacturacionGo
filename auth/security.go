package main

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ---------- Contraseñas ----------
//
// Las contraseñas se guardan con PBKDF2-SHA256 y una sal aleatoria:
//   pbkdf2-sha256$<iteraciones>$<sal base64>$<hash base64>

const iteraciones = 100_000

func hashPassword(password string) (string, error) {
	sal := make([]byte, 16)
	if _, err := rand.Read(sal); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, sal, iteraciones, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", iteraciones, enc.EncodeToString(sal), enc.EncodeToString(key)), nil
}

func checkPassword(password, guardado string) bool {
	partes := strings.Split(guardado, "$")
	if len(partes) != 4 || partes[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(partes[1])
	if err != nil {
		return false
	}
	enc := base64.RawStdEncoding
	sal, err1 := enc.DecodeString(partes[2])
	esperado, err2 := enc.DecodeString(partes[3])
	if err1 != nil || err2 != nil {
		return false
	}
	key, err := pbkdf2.Key(sha256.New, password, sal, iter, len(esperado))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(key, esperado) == 1
}

// ---------- Tokens (JWT HS256) ----------

// Claims es el contenido del token que recibe el frontend tras el login.
type Claims struct {
	Sub    int    `json:"sub"`
	Nombre string `json:"nombre"`
	Email  string `json:"email"`
	Rol    string `json:"rol"`
	Iat    int64  `json:"iat"`
	Exp    int64  `json:"exp"`
}

var b64 = base64.RawURLEncoding

func firmar(datos string, secreto []byte) string {
	mac := hmac.New(sha256.New, secreto)
	mac.Write([]byte(datos))
	return b64.EncodeToString(mac.Sum(nil))
}

func crearToken(u Usuario, secreto []byte, duracion time.Duration) (string, error) {
	ahora := time.Now()
	header := b64.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(Claims{
		Sub: u.ID, Nombre: u.Nombre, Email: u.Email, Rol: u.Rol,
		Iat: ahora.Unix(), Exp: ahora.Add(duracion).Unix(),
	})
	if err != nil {
		return "", err
	}
	datos := header + "." + b64.EncodeToString(payload)
	return datos + "." + firmar(datos, secreto), nil
}

func validarToken(token string, secreto []byte) (*Claims, error) {
	partes := strings.Split(token, ".")
	if len(partes) != 3 {
		return nil, errors.New("token mal formado")
	}
	datos := partes[0] + "." + partes[1]
	if !hmac.Equal([]byte(firmar(datos, secreto)), []byte(partes[2])) {
		return nil, errors.New("firma del token inválida")
	}
	payload, err := b64.DecodeString(partes[1])
	if err != nil {
		return nil, errors.New("token mal formado")
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, errors.New("token mal formado")
	}
	if time.Now().Unix() > c.Exp {
		return nil, errors.New("el token expiró, inicie sesión de nuevo")
	}
	return &c, nil
}
