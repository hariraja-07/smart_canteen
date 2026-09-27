package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

const usage = `smart_canteen backend

Usage:
  smart_canteen [serve]              start the HTTP server (default)
  smart_canteen createuser [flags]   create one account
  smart_canteen createusers          create many accounts from stdin
  smart_canteen reconcile             verify coin balances against the ledger

createuser flags:
  -email     address, must be unique
  -name      display name
  -role      admin | student | staff | canteen_management
  -coins     starting coin balance, credited as an exchange_in ledger entry
  -password  if omitted, read one line from stdin

createusers reads from stdin:
  line 1        the shared password for every record
  lines 2..n    email,name,role[,coins]   (# comments and blanks ignored)

Environment:
  DATABASE_URL   required, postgres connection URL
  JWT_SECRET     required by serve, at least 32 characters

Examples:
  smart_canteen createuser -email ravi@college.edu -name Ravi -role student -coins 100
  echo hunter2hunter2 | smart_canteen createuser -email admin@college.edu -name Admin -role admin
  cat students.csv | smart_canteen createusers

students.csv holds one email,name,role,coins record per line, for example:
  ravi@college.edu,Ravi,student,100
  priya@college.edu,Priya,student,60
`

func runCLI(args []string) int {
	if len(args) == 0 {
		fmt.Print(usage)
		return 2
	}
	switch args[0] {
	case "serve":
		return runServe()
	case "createuser":
		return runCreateUser(args[1:])
	case "createusers":
		return runCreateUsers(args[1:])
	case "reconcile":
		return runReconcile()
	case "-h", "--help", "help":
		fmt.Print(usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		fmt.Print(usage)
		return 2
	}
}

func runCreateUser(args []string) int {
	fs := flag.NewFlagSet("createuser", flag.ContinueOnError)
	email := fs.String("email", "", "email address")
	name := fs.String("name", "", "display name")
	role := fs.String("role", RoleStudent, "admin | student | staff | canteen_management")
	coins := fs.Int("coins", 0, "starting coin balance")
	password := fs.String("password", "", "password, read from stdin when empty")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	pw := *password
	if pw == "" {
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintln(os.Stderr, "error: no password given on stdin")
			return 1
		}
		pw = strings.TrimRight(line, "\r\n")
	}

	if *email == "" || *name == "" {
		fmt.Fprintln(os.Stderr, "error: -email and -name are required")
		return 2
	}

	db, err := initDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer db.Close()

	user, err := createUser(db, *name, *email, pw, *role, *coins)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		if isUniqueViolation(err) {
			return 1
		}
		if errors.Is(err, errInvalidRole) {
			return 2
		}
		return 1
	}

	fmt.Printf("created %s <%s> role=%s coins=%d\n", user.Name, user.Email, user.Role, user.CoinBalance)
	return 0
}

// runCreateUsers provisions many accounts from stdin, one "email,name,role,coins"
// record per line. Blank lines and lines starting with # are ignored. This is
// the bulk path while the app has no admin screen for it yet.
func runCreateUsers(args []string) int {
	fs := flag.NewFlagSet("createusers", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	db, err := initDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer db.Close()

	// One shared password for the batch, supplied on stdin before the records.
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(os.Stderr, "error: expected the shared password on the first line")
		return 1
	}
	password := strings.TrimRight(line, "\r\n")

	created, failed := 0, 0
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		record := strings.TrimSpace(scanner.Text())
		if record == "" || strings.HasPrefix(record, "#") {
			continue
		}
		email, name, role, coins, err := parseUserRecord(record)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %q: %v\n", record, err)
			failed++
			continue
		}
		u, err := createUser(db, name, email, password, role, coins)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", email, err)
			failed++
			continue
		}
		fmt.Printf("created %s <%s> role=%s coins=%d\n", u.Name, u.Email, u.Role, u.CoinBalance)
		created++
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "error reading records: %v\n", err)
		return 1
	}
	fmt.Printf("\n%d created, %d failed\n", created, failed)
	return 0
}

func parseUserRecord(record string) (email, name, role string, coins int, err error) {
	parts := strings.Split(record, ",")
	if len(parts) < 3 {
		return "", "", "", 0, errors.New("expected email,name,role[,coins]")
	}
	email = strings.TrimSpace(parts[0])
	name = strings.TrimSpace(parts[1])
	role = strings.TrimSpace(parts[2])
	if len(parts) > 3 {
		if _, err := fmt.Sscanf(strings.TrimSpace(parts[3]), "%d", &coins); err != nil {
			return "", "", "", 0, errors.New("coins must be a number")
		}
	}
	if err := validRole(role); err != nil {
		return "", "", "", 0, err
	}
	if email == "" || name == "" {
		return "", "", "", 0, errors.New("email and name are required")
	}
	return email, name, role, coins, nil
}
