package sda

import (
	"bytes"
	"github.com/BurntSushi/toml"
	"github.com/dedis/cothority/lib/dbg"
	"io/ioutil"
	"os"
	"path/filepath"
)

/*
 * Writes any structure to a toml-file
 *
 * Takes a filename and an optional directory-name.
 */
func WriteTomlConfig(conf interface{}, filename string, dirOpt ...string) {
	buf := new(bytes.Buffer)
	if err := toml.NewEncoder(buf).Encode(conf); err != nil {
		dbg.Fatal(err)
	}
	err := ioutil.WriteFile(getFullName(filename, dirOpt...), buf.Bytes(), 0660)
	if err != nil {
		dbg.Fatal(err)
	}
}

/*
 * Reads any structure from a toml-file
 *
 * Takes a filename and an optional directory-name
 */
func ReadTomlConfig(conf interface{}, filename string, dirOpt ...string) error {
	buf, err := ioutil.ReadFile(getFullName(filename, dirOpt...))
	if err != nil {
		pwd, _ := os.Getwd()
		dbg.Lvl1("Didn't find", filename, "in", pwd)
		return err
	}

	_, err = toml.Decode(string(buf), conf)
	if err != nil {
		dbg.Fatal(err)
	}

	return nil
}

/*
 * Gets filename and dirname
 *
 * special cases:
 * - filename only
 * - filename in relative path
 * - filename in absolute path
 * - filename and additional path
 */
func getFullName(filename string, dirOpt ...string) string {
	dir := filepath.Dir(filename)
	if len(dirOpt) > 0 {
		dir = dirOpt[0]
	} else {
		if dir == "" {
			dir = "."
		}
	}
	return dir + "/" + filepath.Base(filename)
}
