package main

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Game struct {
	ID        uint   `gorm:"primaryKey"`
	ShortName string `gorm:"uniqueIndex"`
	RepoURL   string
	GitURL    string
	CreatedAt time.Time

	Versions []GameVersion
}

type GameVersion struct {
	ID          uint   `gorm:"primaryKey"`
	GameID      uint   `gorm:"index"`
	VersionHash string `gorm:"uniqueIndex"`
	VersionMD5  string
	FullName    string
	Published   bool `gorm:"default:true;index"`
	CreatedAt   time.Time
}

type File struct {
	ID     uint   `gorm:"primaryKey"`
	MD5Sum string `gorm:"index"`
	CRC32  uint32 `gorm:"index"`
	Len    uint64
}

type VersionFile struct {
	ID            uint   `gorm:"primaryKey"`
	GameVersionID uint   `gorm:"index"`
	FileID        uint   `gorm:"index"`
	Path          string `gorm:"index"`
}

type FileP struct {
	ID     uint
	MD5Sum string
	CRC32  uint32
	Len    uint64
	Path   string
}

type Admin struct {
	ID uint `gorm:"primaryKey"`

	Email string `gorm:"uniqueIndex;not null"`

	PasswordHash string `gorm:"not null"`

	// Two Factor Auth
	TwoFactorEnabled bool
	TwoFactorSecret  string // base32 secret for TOTP

	// Password Reset
	ResetToken       string
	ResetTokenExpiry *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (a *Admin) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	a.PasswordHash = string(hash)
	return nil
}

func (a *Admin) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password))
	return err == nil
}
