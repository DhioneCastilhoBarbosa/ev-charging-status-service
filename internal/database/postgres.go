
package database

import (
"log"
"github.com/jmoiron/sqlx"
_ "github.com/jackc/pgx/v5/stdlib"
)

func ConnectPostgres(url string) *sqlx.DB {

db, err := sqlx.Connect("pgx", url)

if err != nil {
log.Fatal(err)
}

db.SetMaxOpenConns(50)
db.SetMaxIdleConns(25)

return db
}
