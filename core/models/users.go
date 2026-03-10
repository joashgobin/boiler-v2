package models

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/joashgobin/boiler-v2/helpers"
	"golang.org/x/crypto/bcrypt"
)

type UserModelInterface interface {
	Create(name, email, password string) error
	UpdatePassword(email, password string) error
	authenticate(email, password string) (User, error)
	LoginAs(store *session.Store, c fiber.Ctx, email, password string) (User, error)
	emailAuthenticate(email string) (User, error)
	EmailLoginAs(store *session.Store, c fiber.Ctx, email string) (User, error)
	Logout(store *session.Store, c fiber.Ctx) error
	Exists(email string) (bool, error)
	AssignRole(email, role string) error
	RemoveRole(email, role string) error
	ParseFromCSV(path string) error
	GetAll(limit int) []User
}

type User struct {
	ID             int
	Name           string
	Email          string
	Roles          string
	HashedPassword []byte
	Created        time.Time
}

type UserModel struct {
	DB *sql.DB
}

func NewUserModel(db *sql.DB) *UserModel {
	return &UserModel{
		DB: db,
	}
}

var _ UserModelInterface = (*UserModel)(nil)

func (m *UserModel) GetAll(limit int) []User {
	var users []User
	query := `
	SELECT id,name,email,roles,created FROM users
	LIMIT ?
	`
	rows, err := m.DB.Query(query, limit)
	if err != nil {
		log.Errorf("get all users exec error: %v", err)
		return users
	}
	for rows.Next() {
		var user User
		err = rows.Scan(&user.ID, &user.Name, &user.Email, &user.Roles, &user.Created)
		if err != nil {
			log.Errorf("get all users scan error: %v", err)
		}
		users = append(users, user)
	}
	return users
}

func (m *UserModel) ParseFromCSV(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	csvReader := csv.NewReader(file)
	records, err := csvReader.ReadAll()
	if err != nil {
		return err
	}
	for _, record := range records {
		if len(record) >= 3 {
			m.Create(record[0], record[1], "")
			for _, role := range strings.Split(record[2], ";") {
				m.AssignRole(record[1], role)
			}
		}
	}
	return nil
}

func InitUsers(db *sql.DB, appName string) {
	helpers.RunMigration(strings.ReplaceAll(`
	USE <appName>;

CREATE TABLE IF NOT EXISTS users (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL,
    roles VARCHAR(255) NOT NULL,
    hashed_password CHAR(60) NOT NULL,
    created DATETIME NOT NULL,
	UNIQUE KEY users_uc_email (email)
);

	`, "<appName>", appName), db)
}

func (m *UserModel) Create(name, email, password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	stmt := `
	INSERT INTO users (name, email, roles, hashed_password, created)
    VALUES(?, ?, ?, ?, UTC_TIMESTAMP())
	`
	_, err = m.DB.Exec(stmt, name, email, "|user|", string(hashedPassword))
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) {
			if mySQLError.Number == 1062 && strings.Contains(mySQLError.Message, "users_uc_email") {
				return ErrDuplicateEmail
			}
		}
		return err
	}
	return nil
}

func (m *UserModel) UpdatePassword(email, password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("update password generate hash error: %v", err)
	}
	query := `
	UPDATE users
	SET hashed_password = ?
	WHERE email = ?
	`
	_, err = m.DB.Exec(query, hashedPassword, email)
	if err != nil {
		return fmt.Errorf("update password exec error: %v", err)
	}
	return nil
}

func (m *UserModel) AssignRole(email, role string) error {
	selectStmt := `SELECT id, roles FROM users WHERE email = ?`

	var roles string
	var id int

	err := m.DB.QueryRow(selectStmt, email).Scan(&id, &roles)

	if err != nil {
		return err
	}

	newRoles := roles
	if !strings.Contains(roles, "|"+role+"|") {
		newRoles += "|" + role + "|"

		log.Infof("setting new roles to: %s", newRoles)

		updateStmt := `UPDATE users
		SET roles = ?
		WHERE email = ?
		`
		result, err := m.DB.Exec(updateStmt, newRoles, email)
		if err != nil {
			return err
		}
		_, err = result.RowsAffected()
		if err != nil {
			return err
		}
		log.Infof("updated roles to: %s", newRoles)
	}

	return nil
}

func (m *UserModel) RemoveRole(email, role string) error {
	selectStmt := `SELECT id, roles FROM users WHERE email = ?`

	var roles string
	var id int

	err := m.DB.QueryRow(selectStmt, id).Scan(&id, &roles)

	if err != nil {
		return err
	}

	log.Infof("%s has roles: %s", email, roles)

	newRoles := roles
	if strings.Contains(roles, "|"+role+"|") {
		newRoles = strings.ReplaceAll(newRoles, "|"+role+"|", "")

		log.Infof("setting new roles to: %s", newRoles)

		updateStmt := `UPDATE users
		SET roles = ?
		WHERE email = ?
		`
		result, err := m.DB.Exec(updateStmt, newRoles, email)
		if err != nil {
			return err
		}
		_, err = result.RowsAffected()
		if err != nil {
			return err
		}
		log.Infof("updated roles to: %s", newRoles)
	}

	return nil
}

func (m *UserModel) emailAuthenticate(email string) (User, error) {
	var user User
	stmt := "SELECT id, name, roles FROM users WHERE email = ?"
	err := m.DB.QueryRow(stmt, email).Scan(&user.ID, &user.Name, &user.Roles)
	if err != nil {
		return User{}, err
	}
	user.Email = email
	return user, nil
}

func (m *UserModel) authenticate(email, password string) (User, error) {
	var user User
	stmt := "SELECT id, name, roles, hashed_password FROM users WHERE email = ?"
	err := m.DB.QueryRow(stmt, email).Scan(&user.ID, &user.Name, &user.Roles, &user.HashedPassword)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user, ErrInvalidCredentials
		} else {
			return user, err
		}
	}
	err = bcrypt.CompareHashAndPassword(user.HashedPassword, []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return user, ErrInvalidCredentials
		} else {
			return user, err
		}
	}
	user.Email = email
	return user, nil
}

func (m *UserModel) EmailLoginAs(store *session.Store, c fiber.Ctx, email string) (User, error) {
	user, err := m.emailAuthenticate(email)
	if err != nil {
		return User{}, fmt.Errorf("credentials error: %v", err)
	}
	if err != nil {
		return User{}, fmt.Errorf("credentials error: %v", err)
	}

	sess, err := store.Get(c)
	if err != nil {
		return User{}, fmt.Errorf("get session error: %v", err)
	}
	defer sess.Release()
	if err := sess.Reset(); err != nil {
		return User{}, fmt.Errorf("reset session error: %v", err)
	}

	sess.Set("user", user)
	// sess.SetIdleTimeout(m.sessionIdleTimeout)

	if err := sess.Save(); err != nil {
		return User{}, fmt.Errorf("save session error: %v", err)
	}
	return user, nil
}

func (m *UserModel) LoginAs(store *session.Store, c fiber.Ctx, email, password string) (User, error) {
	user, err := m.authenticate(email, password)
	if err != nil {
		return User{}, fmt.Errorf("credentials error: %v", err)
	}

	sess, err := store.Get(c)
	if err != nil {
		return User{}, fmt.Errorf("get session error: %v", err)
	}
	defer sess.Release()
	if err := sess.Reset(); err != nil {
		return User{}, fmt.Errorf("reset session error: %v", err)
	}

	sess.Set("user", user)
	// sess.SetIdleTimeout(m.sessionIdleTimeout)

	if err := sess.Save(); err != nil {
		return User{}, fmt.Errorf("save session error: %v", err)
	}

	return user, nil
}

func (m *UserModel) Logout(store *session.Store, c fiber.Ctx) error {
	session, err := store.Get(c)
	if err != nil {
		return err
	}
	defer session.Release()

	if err := session.Destroy(); err != nil {
		return err
	}
	return nil
}

func (m *UserModel) Exists(email string) (bool, error) {
	var exists bool
	stmt := "SELECT EXISTS(SELECT true FROM users WHERE email = ?)"
	err := m.DB.QueryRow(stmt, email).Scan(&exists)
	return exists, err
}

func RequireRole(store *session.Store, flash helpers.FlashInterface, role string) fiber.Handler {
	return func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		defer sess.Release()
		if err != nil {
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		user, ok := sess.Get("user").(User)

		// redirect if user value is not set in session
		if !ok {
			flash.Push(c, "You need to be logged in")
			return c.Redirect().To("/login")
		}

		// redirect if user roles are not defined in session
		roles := user.Roles
		if roles == "" {
			flash.Push(c, "You need to be logged in")
			return c.Redirect().To("/login")
		}

		// redirect if user session does not specify the required role
		if !strings.Contains(roles, "|"+role+"|") {
			flash.Push(c, fmt.Sprintf("You need to be logged in as %s", role))
			return c.Redirect().To("/")
		}
		return c.Next()
	}
}
