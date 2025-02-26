package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kingpin/v2"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/projectdiscovery/gologger"
)

type UpdateCommand struct {
	Force bool
}

type SearchCommand struct {
	Export      bool
	Keyword     string
	Proxy       string
	UpdateDb    bool
	JsonOutput  bool
	TableOutput bool
}

type Credential struct {
	ProductVendor string `json:"product_vendor"`
	Username      string `json:"username"`
	Password      string `json:"password"`
}

func (c Credential) Row() table.Row {
	return table.Row{c.ProductVendor, c.Username, c.Password}
}

func (c Credential) Headers() table.Row {
	return table.Row{"Product Vendor", "Username", "Password"}
}

var (
	cli                 = kingpin.New("creds", "creds helps you find default credentials")
	homeDir, homeDirErr = os.UserHomeDir()
	credsConfigDir      = filepath.Join(homeDir, ".config", "creds")
)

const (
	DefaultCredsCSVFileUrl     = "https://raw.githubusercontent.com/ihebski/DefaultCreds-cheat-sheet/main/DefaultCreds-Cheat-Sheet.csv"
	CredentialDatabaseFileName = "credential_database.json"
)

func init() {
	search := &SearchCommand{}
	searchCommand := cli.Command("search", "search credentials").Action(search.run)
	searchCommand.Arg("keyword", "search product").Required().StringVar(&search.Keyword)
	searchCommand.Flag("proxy", "proxy").Short('p').StringVar(&search.Proxy)
	searchCommand.Flag("export", "export data").Short('e').BoolVar(&search.Export)
	searchCommand.Flag("update-db", "update database before searching").Short('u').Default("false").BoolVar(&search.UpdateDb)
	searchCommand.Flag("json", "json output").Short('j').Default("false").BoolVar(&search.JsonOutput)
	searchCommand.Flag("table", "table output").Short('t').Default("false").BoolVar(&search.TableOutput)

	update := &UpdateCommand{}
	updateCommand := cli.Command("update", "update database").Action(update.run)
	updateCommand.Flag("force", "force update").Short('f').Default("true").BoolVar(&update.Force)
}

func ensureConfigFolder() {
	if homeDirErr != nil {
		gologger.Fatal().Msgf("could not retrieve config directory. error: %s\n", homeDirErr)
		return
	}

	folderInfo, err := os.Stat(credsConfigDir)
	if os.IsNotExist(err) || err != nil || !folderInfo.IsDir() {
		_ = os.MkdirAll(credsConfigDir, 0700)
	}
}

func (u *UpdateCommand) run(c *kingpin.ParseContext) error {
	gologger.Silent().Msg("updating database")

	ensureConfigFolder()

	res, err := http.Get(DefaultCredsCSVFileUrl)
	if err != nil {
		gologger.Error().Msgf("could not download database. error: %s\n", err)
		return err
	}
	defer res.Body.Close()

	rows, err := csv.NewReader(res.Body).ReadAll()
	if err != nil {
		gologger.Error().Msgf("could not read response body. error: %s\n", err)
		return err
	}
	db := make([]Credential, 0)

	for _, row := range rows {
		if len(row) != 3 {
			gologger.Error().
				Str("row", strings.Join(row, ",")).
				Msg("invalid entry\n")
			continue
		}
		db = append(db, Credential{
			ProductVendor: row[0],
			Username:      row[1],
			Password:      row[2],
		})
	}

	fileDbPath := filepath.Join(credsConfigDir, CredentialDatabaseFileName)

	// always remove and create again
	_ = os.Remove(fileDbPath)

	fileDb, err := os.Create(fileDbPath)
	if err != nil {
		gologger.Error().Msgf("could not create database file. error: %s\n", err)
		return nil
	}

	err = json.NewEncoder(fileDb).Encode(db)
	if err != nil {
		gologger.Error().Msgf("could not write credential database. error: %s\n", err)
		return nil
	}

	return nil
}

func ensureDatabase() {
	ensureConfigFolder()

	dbFileLocation := filepath.Join(credsConfigDir, CredentialDatabaseFileName)
	fileInfo, err := os.Stat(dbFileLocation)
	if os.IsNotExist(err) || err != nil || fileInfo.Size() == 0 {
		u := &UpdateCommand{}
		_ = u.run(nil)
	}
}

func readDatabase() []Credential {
	dbFileLocation := filepath.Join(credsConfigDir, CredentialDatabaseFileName)
	dbRaw, err := os.ReadFile(dbFileLocation)
	if err != nil {
		gologger.Fatal().Msgf("could not read database. error: %s\n", err)
	}

	db := make([]Credential, 0)
	err = json.Unmarshal(dbRaw, &db)
	if err != nil {
		gologger.Fatal().Msgf("could not parse database. error: %s\n", err)
	}

	return db
}

func configureOutput(s *SearchCommand) {
	if !s.TableOutput && !s.JsonOutput {
		s.TableOutput = true
	}
}

func (s *SearchCommand) run(c *kingpin.ParseContext) error {
	configureOutput(s)
	if s.UpdateDb {
		u := &UpdateCommand{}
		_ = u.run(nil)
	}

	ensureDatabase()

	db := readDatabase()
	res := make([]Credential, 0)
	for _, cred := range db {
		if strings.Contains(
			strings.ToLower(cred.ProductVendor),
			strings.ToLower(s.Keyword),
		) {
			res = append(res, cred)
		}
	}

	if len(res) == 0 {
		gologger.Silent().Msg("no credentials found with search keyword\n")
		return nil
	}

	if s.TableOutput {
		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendHeader(Credential{}.Headers())

		for _, r := range res {
			t.AppendRow(r.Row())
		}
		t.Render()
	}

	if s.JsonOutput {
		resByte, err := json.Marshal(res)
		if err != nil {
			return errors.New("could not parse search result")
		}

		_, _ = os.Stdout.Write(resByte)
	}

	return nil
}

func main() {
	kingpin.MustParse(cli.Parse(os.Args[1:]))
}
