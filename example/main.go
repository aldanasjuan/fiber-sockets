package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aldanasjuan/sockets"
	"github.com/gofiber/fiber"
)

func main() {
	app := fiber.New()
	mainHub := sockets.NewMainHub()
	go mainHub.Run(handler)
	app.Use("/ws", addLocals)
	app.Get("/ws", sockets.NewReader(mainHub))
	app.Listen(":3000")
}

func addLocals(c *fiber.Ctx) error {
	id := c.Query("id")
	c.Locals("socketsID", id)
	user := map[string]interface{}{"name": "Juan", "userID": 123}
	c.Locals("socketsData", user)
	return c.Next()
}

func handler(r sockets.Request, write func(interface{})) {
	start := time.Now()
	in := map[string]interface{}{}
	json.Unmarshal(r.Msg, &in)
	in["id"] = r.ID
	fmt.Printf("\nReceived request %v:\n\nWriting data: %+v", r.ID, in)
	write(in)
	fmt.Println("Elapsed", time.Since(start))
}
