package main

import (
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func CreateSampleAdmin() error {
	email := "admin@techa-rts.com"
	password := "admin123"

	var existing Admin
	err := DB.Where("email = ?", email).First(&existing).Error
	if err == nil {
		// already exists
		return nil
	}

	admin := Admin{
		Email: email,
	}

	if err := admin.SetPassword(password); err != nil {
		return err
	}

	admin.TwoFactorEnabled = false

	return DB.Create(&admin).Error
}

func InitDB(cfg Config) {
	var err error
	DB, err = gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		log.Fatal("failed to connect database:", err)
	}

	err = DB.AutoMigrate(&Game{}, &GameVersion{}, &File{}, &VersionFile{}, &Admin{})
	if err != nil {
		log.Fatal("failed to migrate:", err)
	}

	CreateSampleAdmin()
}
