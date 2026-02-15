package main

import (
	"compress/gzip"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"
)

func ReposHandler(c *gin.Context) {
	c.Header("Content-Type", "application/gzip")

	gz := gzip.NewWriter(c.Writer)
	defer gz.Close()

	var games []Game
	DB.Find(&games)

	for _, g := range games {
		line := fmt.Sprintf("%s,%s,,\n", g.ShortName, g.RepoURL)
		gz.Write([]byte(line))
	}
}

func VersionsHandler(c *gin.Context) {
	shortname := c.Param("shortname")

	var game Game
	if err := DB.Where("short_name = ?", shortname).First(&game).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	var versions []GameVersion
	DB.Where("game_id = ?", game.ID).Find(&versions)

	c.Header("Content-Type", "application/gzip")

	gz := gzip.NewWriter(c.Writer)
	defer gz.Close()

	for _, v := range versions {
		line := fmt.Sprintf("%s:%s,%s,,%s\n",
			shortname,
			v.VersionHash,
			v.VersionMD5,
			v.FullName,
		)
		gz.Write([]byte(line))
	}
}

func GetSDPRecords(tx *gorm.DB, md5 string) ([]SdpRecord, error) {
	var version GameVersion
	if err := tx.Where("version_md5 = ?", md5).First(&version).Error; err != nil {

		return make([]SdpRecord, 0), fmt.Errorf("Version not found")
	}

	records := make([]SdpRecord, 0)

	var files []FileP

	err := tx.Table("version_files").Select("* , files.*").Joins("INNER JOIN files ON version_files.file_id = files.id").Where("version_files.game_version_id = ?", version.ID).Order("files.crc32").Scan(&files).Error

	if err != nil {
		log.Println(err.Error())
		return make([]SdpRecord, 0), fmt.Errorf("Version corrupted: %s", err.Error())
	}

	for _, vf := range files {
		h, _ := hex.DecodeString(vf.MD5Sum)
		records = append(records, SdpRecord{
			Filename: vf.Path,
			MD5:      [16]byte(h),
			CRC32:    vf.CRC32,
			Size:     uint32(vf.Len),
		})
	}
	return records, nil

}

func GetSDPMD5(tx *gorm.DB, h string) string {
	records, _ := GetSDPRecords(tx, h)
	md5hash := md5.New()
	for _, r := range records {
		nameMd5 := md5.New()
		nameMd5.Write([]byte(r.Filename))
		h1 := nameMd5.Sum(nil)
		md5hash.Write(h1)
		md5hash.Write(r.MD5[:])
	}

	//WriteAllFileRecords(md5hash, records)
	return hex.EncodeToString(md5hash.Sum(nil))
}

func PackageHandler(c *gin.Context) {
	//shortname := c.Param("shortname")

	filename := c.Param("filename")

	x := strings.Split(filename, ".")

	records, err := GetSDPRecords(DB, x[0])

	c.Header("Content-Type", "application/octet-stream")

	gz := gzip.NewWriter(c.Writer)
	defer gz.Close()

	err = WriteAllFileRecords(gz, records)

	if err != nil {
		log.Println(err.Error())
		//c.Status(http.StatusInternalServerError)
		return
	}

}

func GetBit(data []byte, bitIndex int) bool {
	byteIndex := bitIndex / 8
	bitPos := bitIndex % 8

	if byteIndex >= len(data) {
		return false
	}

	return (data[byteIndex] & (1 << bitPos)) != 0
}

func StreamerHandler(c *gin.Context) {
	cfg, _ := LoadConfig()
	records, err := GetSDPRecords(DB, c.Request.URL.RawQuery)
	if err != nil {
		log.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		return
	}

	rdr, _ := gzip.NewReader(c.Request.Body)
	defer rdr.Close()

	req, err := io.ReadAll(rdr)

	if err != nil {
		log.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		return
	}
	totallen := 0
	for index, r := range records {
		if GetBit(req, index) {
			fp := computeAndCreatePoolPath(cfg, hex.EncodeToString(r.MD5[:]))
			st, err := os.Stat(fp)
			if err != nil {
				panic(err)
			}
			totallen += int(st.Size()) + 4
		}
	}
	//indexes := make([]uint16, 0)
	c.Header("Content-Length", fmt.Sprintf("%d", totallen))
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Type", "application/octet-stream")
	c.Status(http.StatusOK)

	c.Stream(func(w io.Writer) bool {
		for index, r := range records {
			if GetBit(req, index) {
				fp := computeAndCreatePoolPath(cfg, hex.EncodeToString(r.MD5[:]))
				st, err := os.Stat(fp)
				if err != nil {
					panic(err)
				}
				srcfile, err := os.Open(fp)
				if err != nil {
					panic(err)
				}
				defer srcfile.Close()
				var s uint32 = uint32(st.Size())
				binary.Write(w, binary.BigEndian, &s)
				io.Copy(w, srcfile)
				//log.Printf("Streaming %s %d\n", fp, st.Size())
			}

		}
		return false
	})

	//fmt.Println(indexes)
}

func CreateGame(c *gin.Context) {
	var game Game

	game.ShortName = c.PostForm("short_name")
	game.RepoURL = c.PostForm("repo_url")
	game.GitURL = c.PostForm("git_url")

	DB.Create(&game)

	c.Redirect(http.StatusFound, "/admin/games")
}

func ListVersions(c *gin.Context) {
	id := c.Param("id")

	var game Game
	DB.Preload("Versions").First(&game, id)

	c.HTML(http.StatusOK, "versions.html", gin.H{
		"game": game,
	})
}

func UnpublishVersion(c *gin.Context) {
	id := c.Param("id")

	DB.Model(&GameVersion{}).
		Where("id = ?", id).
		Update("published", false)

	c.Redirect(http.StatusFound, c.Request.Referer())
}

func ListGames(c *gin.Context) {
	var games []Game
	DB.Preload("Versions").Find(&games)

	c.HTML(http.StatusOK, "games.html", gin.H{
		"games": games,
	})
}

func ShowLogin(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", gin.H{
		"error": "",
	})
}

func HandleLogin(c *gin.Context) {
	email := c.PostForm("email")
	password := c.PostForm("password")
	code := c.PostForm("code") // 2FA code (optional)

	var admin Admin
	if err := DB.Where("email = ?", email).First(&admin).Error; err != nil {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"error": "Invalid credentials",
		})
		return
	}

	if !admin.CheckPassword(password) {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"error": "Invalid credentials",
		})
		return
	}

	// 2FA validation
	if admin.TwoFactorEnabled {
		if !totp.Validate(code, admin.TwoFactorSecret) {
			c.HTML(http.StatusUnauthorized, "login.html", gin.H{
				"error": "Invalid 2FA code",
			})
			return
		}
	}

	session := sessions.Default(c)
	session.Set("admin_id", admin.ID)
	session.Save()

	c.Redirect(http.StatusFound, "/admin")
}

func Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()

	c.Redirect(http.StatusFound, "/admin/login")
}

func Dashboard(c *gin.Context) {
	var gameCount int64
	var versionCount int64
	var publishedCount int64

	DB.Model(&Game{}).Count(&gameCount)
	DB.Model(&GameVersion{}).Count(&versionCount)
	DB.Model(&GameVersion{}).Where("published = ?", true).Count(&publishedCount)

	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"gameCount":      gameCount,
		"versionCount":   versionCount,
		"publishedCount": publishedCount,
	})
}

func ShowNewGame(c *gin.Context) {
	c.HTML(http.StatusOK, "new_game.html", nil)
}
