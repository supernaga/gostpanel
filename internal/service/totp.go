package service

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"

	"github.com/pquerna/otp/totp"
)

// GenerateTOTPSecret 生成 TOTP 密钥和 QR 码
func (s *Service) GenerateTOTPSecret(username string) (secret string, qrBase64 string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "GOST Panel",
		AccountName: username,
	})
	if err != nil {
		return "", "", err
	}

	// 生成 QR 码 base64
	var buf bytes.Buffer
	img, err := key.Image(200, 200)
	if err != nil {
		return "", "", err
	}
	png.Encode(&buf, img)
	qrBase64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	return key.Secret(), qrBase64, nil
}

// ValidateTOTP 验证 TOTP 代码
func (s *Service) ValidateTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
}

// GenerateBackupCodes 生成备份码
func (s *Service) GenerateBackupCodes() (codes []string, hashJSON string, err error) {
	codes = make([]string, 8)
	hashes := make([]string, 8)
	for i := 0; i < 8; i++ {
		b := make([]byte, 4)
		if _, err := rand.Read(b); err != nil {
			return nil, "", err
		}
		codes[i] = fmt.Sprintf("%08x", b)
		hash := sha256.Sum256([]byte(codes[i]))
		hashes[i] = fmt.Sprintf("%x", hash[:])
	}
	hashBytes, err := json.Marshal(hashes)
	if err != nil {
		return nil, "", err
	}
	return codes, string(hashBytes), nil
}

// ValidateBackupCode 验证备份码（使用后移除）
func (s *Service) ValidateBackupCode(storedHashesJSON, code string) (bool, string) {
	var hashes []string
	if err := json.Unmarshal([]byte(storedHashesJSON), &hashes); err != nil {
		return false, storedHashesJSON
	}

	codeHash := sha256.Sum256([]byte(code))
	codeHashStr := fmt.Sprintf("%x", codeHash[:])

	for i, h := range hashes {
		if h == codeHashStr {
			// 使用后移除
			hashes = append(hashes[:i], hashes[i+1:]...)
			newJSON, _ := json.Marshal(hashes)
			return true, string(newJSON)
		}
	}
	return false, storedHashesJSON
}
