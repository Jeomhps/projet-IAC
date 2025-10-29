package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/gin-gonic/gin"
)

type Machines struct{ db *db.DB }

func NewMachines(d *db.DB) *Machines { return &Machines{db: d} }

func (h *Machines) List(c *gin.Context) {
	q := "SELECT * FROM machines"
	args := []any{}
	where := []string{}

	if v := c.Query("eligible"); v != "" {
		if strings.EqualFold(v, "true") || v == "1" {
			where = append(where, "(enabled=1 AND online=1 AND reserved=0)")
		} else {
			where = append(where, "NOT (enabled=1 AND online=1 AND reserved=0)")
		}
	}
	if v := c.Query("reserved"); v != "" {
		if strings.EqualFold(v, "true") || v == "1" {
			where = append(where, "reserved=1")
		} else {
			where = append(where, "reserved=0")
		}
	}
	if v := c.Query("name"); v != "" {
		where = append(where, "name LIKE ?")
		args = append(args, v+"%")
	}
	if !c.GetBool("is_admin") {
		where = append(where, "enabled=1")
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY name ASC"

	var ms []db.Machine
	if err := h.db.Select(&ms, q, args...); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	admin := c.GetBool("is_admin")
	out := make([]gin.H, 0, len(ms))
	for _, m := range ms {
		item := gin.H{
			"name":           m.Name,
			"reserved":       m.Reserved,
			"reserved_until": formatTimePtr(m.ReservedUntil),
		}
		if admin {
			item["host"] = m.Host
			item["port"] = m.Port
			item["user"] = m.User
			item["reserved_by"] = m.ReservedBy
			item["enabled"] = m.Enabled
			item["online"] = m.Online
			item["last_seen_at"] = formatTimePtr(m.LastSeenAt)
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, out)
}

func (h *Machines) Get(c *gin.Context) {
	name := c.Param("name")
	var m db.Machine
	if err := h.db.Get(&m, "SELECT * FROM machines WHERE name=?", name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}
	if !c.GetBool("is_admin") && !m.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}
	admin := c.GetBool("is_admin")
	item := gin.H{
		"name": m.Name, "reserved": m.Reserved, "reserved_until": formatTimePtr(m.ReservedUntil),
	}
	if admin {
		item["host"] = m.Host
		item["port"] = m.Port
		item["user"] = m.User
		item["reserved_by"] = m.ReservedBy
		item["enabled"] = m.Enabled
		item["online"] = m.Online
		item["last_seen_at"] = formatTimePtr(m.LastSeenAt)
	}
	c.JSON(http.StatusOK, item)
}

func (h *Machines) Create(c *gin.Context) {
	var in struct {
		Name, Host, User, Password string
		Port                       int
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Name == "" || in.Host == "" || in.User == "" || in.Password == "" || in.Port <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	if _, err := h.db.Exec(`INSERT INTO machines (name,host,port,user,password,reserved,reserved_by,reserved_until,enabled,online)
		VALUES (?,?,?,?,?,0,NULL,NULL,1,1)`, in.Name, in.Host, in.Port, in.User, in.Password); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": "Machine exists"})
		return
	}
	var m db.Machine
	_ = h.db.Get(&m, "SELECT * FROM machines WHERE name=?", in.Name)
	c.JSON(http.StatusCreated, gin.H{
		"name": m.Name, "host": m.Host, "port": m.Port, "user": m.User,
		"reserved": m.Reserved, "reserved_by": m.ReservedBy, "reserved_until": formatTimePtr(m.ReservedUntil),
		"enabled": m.Enabled, "online": m.Online, "last_seen_at": formatTimePtr(m.LastSeenAt),
	})
}

func (h *Machines) Update(c *gin.Context) {
	name := c.Param("name")
	var in map[string]any
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	fields := []string{}
	args := []any{}
	for _, f := range []string{"host", "port", "user", "password", "enabled", "online", "name"} {
		if v, ok := in[f]; ok {
			// ensure port is numeric if present
			if f == "port" {
				switch vv := v.(type) {
				case float64:
					args = append(args, int(vv))
				case string:
					if n, err := strconv.Atoi(vv); err == nil {
						args = append(args, n)
					} else {
						args = append(args, vv)
					}
				default:
					args = append(args, v)
				}
			} else {
				args = append(args, v)
			}
			fields = append(fields, f+"=?")
		}
	}
	if len(fields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "no fields"})
		return
	}
	args = append(args, name)
	if _, err := h.db.Exec("UPDATE machines SET "+strings.Join(fields, ",")+" WHERE name=?", args...); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	h.Get(c)
}

func (h *Machines) Delete(c *gin.Context) {
	name := c.Param("name")
	var reserved bool
	if err := h.db.Get(&reserved, "SELECT reserved FROM machines WHERE name=?", name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}
	if reserved {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "Machine is currently reserved; release first"})
		return
	}
	_, _ = h.db.Exec("DELETE FROM machines WHERE name=?", name)
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
