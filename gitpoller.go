package main

import (
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

func StartGitPoller(cfg Config) {
	go func() {
		for {
			checkRepos(cfg)
			time.Sleep(5 * time.Minute)
		}
	}()
}

func checkRepos(cfg Config) {
	var games []Game
	DB.Find(&games)

	for _, game := range games {
		processGame(cfg, game)
	}
}

var (
	// Strictly match: return { ... }
	blockRegex = regexp.MustCompile(`(?s)^\s*return\s*\{\s*(.*?)\s*\}\s*$`)

	// Strictly match one key/value line
	fieldRegex = regexp.MustCompile(`^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*=\s*(?:'([^']*)'|([0-9]+))\s*,?\s*$`)
)

func parseLuaTable(input string) (map[string]string, error) {
	result := make(map[string]string)

	// 1️⃣ Validate and extract block
	blockMatch := blockRegex.FindStringSubmatch(input)
	if blockMatch == nil {
		return nil, errors.New("input must be a single 'return { ... }' block")
	}

	body := blockMatch[1]

	// 2️⃣ Split into lines and validate each
	lines := strings.Split(body, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		match := fieldRegex.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("invalid line: %s", line)
		}

		key := match[1]

		var value string
		if match[2] != "" {
			value = match[2] // string
		} else {
			value = match[3] // number
		}

		result[key] = value
	}

	return result, nil
}

func processGame(cfg Config, game Game) {
	repoPath := filepath.Join(cfg.ReposPath, game.ShortName)

	// Clone repo if it doesn't exist
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		if err := exec.Command("git", "clone", game.GitURL, repoPath).Run(); err != nil {
			fmt.Println("Failed to clone repo:", err)
			return
		}
	}

	// Fetch latest changes and tags
	if err := exec.Command("git", "-C", repoPath, "fetch", "--tags").Run(); err != nil {
		fmt.Println("Failed to fetch repo:", err)
		return
	}

	// Get the last 30 commit hashes
	cmd := exec.Command("git", "-C", repoPath, "rev-list", fmt.Sprintf("--max-count=%d", cfg.BackLog), "origin/HEAD")
	out, err := cmd.Output()
	if err != nil {
		fmt.Println("Failed to get commit list:", err)
		return
	}

	commits := strings.Split(strings.TrimSpace(string(out)), "\n")

	for _, hash := range commits {
		versionIdentifier := hash

		// Check if there is a tag for this commit
		tagCmd := exec.Command("git", "-C", repoPath, "tag", "--points-at", hash)
		tagOut, err := tagCmd.Output()
		if err != nil {
			fmt.Println("Failed to get tag for commit", hash, ":", err)
			continue
		}
		tag := strings.TrimSpace(string(tagOut))
		if tag != "" {
			versionIdentifier = tag
		}

		// Check if this version already exists in the DB
		var existing GameVersion
		if err := DB.Where("version_hash = ?", "git:"+versionIdentifier).First(&existing).Error; err == nil {
			continue // version already exists
		}

		var istag bool = false

		// Checkout the commit/tag
		checkoutCmd := exec.Command("git", "-C", repoPath, "reset", "--hard", hash)
		if tag != "" {
			checkoutCmd = exec.Command("git", "-C", repoPath, "reset", "--hard", tag)
			istag = true
		}

		if err := checkoutCmd.Run(); err != nil {
			fmt.Println("Failed to checkout", versionIdentifier, ":", err)
			continue
		}

		progCmd := exec.Command("git", "-C", repoPath, "rev-list", "--count", "HEAD")
		progOut, err := progCmd.Output()
		if err != nil {
			fmt.Println("Failed to get count for commit", hash, ":", err)
			continue
		}

		scount := strings.TrimSpace(string(progOut))

		prog, err := strconv.Atoi(scount)

		if err != nil {
			fmt.Println("Failed to parse commit count for commit", hash, ":", scount, err.Error())
			continue
		}

		fmt.Printf("Prog is %d\n", prog)

		modinfo, err := os.Open(filepath.Join(repoPath, "modinfo.lua"))
		fullname := game.ShortName + "-" + hash[:min(8, len(hash))]
		if err == nil {
			modinfocontent, _ := io.ReadAll(modinfo)
			modinfo.Close()
			var newmodinfo string = ""
			if !istag {
				newmodinfo = strings.ReplaceAll(string(modinfocontent), "$VERSION", fmt.Sprintf("test-%d-%s", prog, hash[:min(7, len(hash))]))
			} else {
				newmodinfo = strings.ReplaceAll(string(modinfocontent), "$VERSION", tag)
			}

			values, err := parseLuaTable(newmodinfo)

			if err != nil {
				fmt.Println(err.Error())
				continue
			}

			name, ok := values["name"]
			version, ok2 := values["version"]
			if ok && ok2 {
				fullname = name + " " + version
			}

			out, _ := os.Create(filepath.Join(repoPath, "modinfo.lua"))
			out.Write([]byte(newmodinfo))
			out.Close()

			log.Printf("Overridden version in modinfo\n")
		}

		// Create the version
		createVersion(repoPath, game, versionIdentifier, fullname, prog, cfg)

	}
}
func computeAndCreatePoolPath(cfg Config, md5sum string) string {

	filep := filepath.Join(cfg.PoolPath, md5sum[0:2])

	if _, err := os.Stat(filep); os.IsNotExist(err) {
		log.Printf("Creating pool path %s\n", filep)
	}

	err := os.MkdirAll(filep, 0750)
	if err != nil {
		panic(err)
	}

	return filepath.Join(filep, md5sum[2:]+".gz")

}

func CopyFile(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}

func createVersion(repoPath string, game Game, hash string, fullname string, prog int, cfg Config) {
	err := DB.Transaction(func(tx *gorm.DB) error {

		versionMD5 := md5sumString(hash) //TODO

		version := GameVersion{
			GameID:      game.ID,
			VersionHash: "git:" + hash,
			VersionMD5:  versionMD5,
			FullName:    fullname, //game.ShortName + "-" + hash[:min(7, len(hash))],
			Published:   false,
		}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}

		err := filepath.Walk(repoPath, func(fullpath string, info os.FileInfo, err error) error {
			if info == nil || info.IsDir() {
				return nil
			}

			path, _ := filepath.Rel(repoPath, fullpath)

			path = strings.ToLower(path)

			skip := false

			if strings.HasPrefix(path, ".git") {
				skip = true
			}

			if !skip {

				sums, err := FileSums(fullpath)
				if err != nil {
					return err
				}

				var file File
				if err := tx.Where("md5_sum = ?", sums.MD5hex).First(&file).Error; err != nil {

					file = File{

						MD5Sum: sums.MD5hex,
						CRC32:  sums.CRC32,
						Len:    uint64(info.Size()),
					}
					res := tx.Create(&file)
					if res.Error != nil {
						return res.Error
					}

					pp := computeAndCreatePoolPath(cfg, sums.MD5hex)
					if _, err := os.Stat(pp); os.IsNotExist(err) {
						//_, err = CopyFile(fullpath, pp)
						dest, err := os.Create(pp)
						if err != nil {
							log.Println(err.Error())
							return err
						}
						gzw := gzip.NewWriter(dest)
						src, err := os.Open(fullpath)
						if err != nil {
							dest.Close()
							os.Remove(pp)
							log.Println(err.Error())
							return err
						}

						defer dest.Close()
						defer src.Close()

						_, err = io.Copy(gzw, src)

						gzw.Flush()
						gzw.Close()

						if err != nil {
							log.Println(err.Error())
							return err
						}
					}
				}

				vf := VersionFile{
					GameVersionID: version.ID,
					FileID:        file.ID,
					Path:          path,
				}
				tx.Create(&vf)

			}

			return nil
		})

		newMD5 := GetSDPMD5(tx, versionMD5)
		log.Printf("MD5 %s -> %s\n", versionMD5, newMD5)

		version.VersionMD5 = newMD5
		tx.Save(&version)

		return err
	})

	if err != nil {
		log.Println("Failed creating version:", err)
		return
	}

}

type FileChecksums struct {
	MD5hex string
	MD5    [16]byte
	CRC32  uint32
}

func FileSums(path string) (*FileChecksums, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h := md5.New()
	io.Copy(h, f)

	ret := FileChecksums{}
	ret.MD5 = [16]byte(h.Sum(nil))
	ret.MD5hex = hex.EncodeToString(ret.MD5[:])
	C := crc32.New(crc32.IEEETable)
	f.Seek(0, io.SeekStart)
	io.Copy(C, f)
	ret.CRC32 = C.Sum32()

	return &ret, nil
}

func md5sumString(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
