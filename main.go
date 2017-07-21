package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/labstack/echo"

	yaml "gopkg.in/yaml.v2"
)

type Application struct {
	Listen string  `yaml:"listen,omitempty"`
	Routes []Route `yaml:"routes,omitempty"`
}

type Route struct {
	Path   string `yaml:"path,omitempty"`
	Method string `yaml:"method,omitempty"`
	Upload string `yaml:"upload,omitempty"`
	Exec   string `yaml:"exec,omitempty"`
}

func handler(route Route) func(echo.Context) error {
	return func(c echo.Context) error {
		args := []string{"-c"}
		args = append(args, route.Exec)

		cmd := exec.Command("sh", args...)
		cmd.Env = []string{}

		for _, name := range c.ParamNames() {
			cmd.Env = append(cmd.Env, fmt.Sprintf("PARAM_%s=%s", strings.ToUpper(name), c.Param(name)))
		}

		for name, values := range c.QueryParams() {
			cmd.Env = append(cmd.Env, fmt.Sprintf("QUERY_%s=%s", strings.ToUpper(name), values[0]))
		}

		for key, value := range c.Request().Header {
			cmd.Env = append(cmd.Env, fmt.Sprintf("HEADER_%s=%s", strings.ToUpper(key), value))
		}

		if route.Upload != "" {
			upload, err := c.FormFile(route.Upload)
			if err != nil {
				return c.NoContent(http.StatusInternalServerError)
			}

			src, err := upload.Open()
			if err != nil {
				return c.NoContent(http.StatusInternalServerError)
			}
			defer src.Close()

			if err = os.MkdirAll("uploads", 0700); err != nil {
				return c.NoContent(http.StatusInternalServerError)
			}

			dst, err := os.Create(path.Join("uploads", upload.Filename))
			if err != nil {
				fmt.Println(err)
				return c.NoContent(http.StatusInternalServerError)
			}
			defer dst.Close()

			defer func() {
				os.Remove(dst.Name())
			}()

			if _, err = io.Copy(dst, src); err != nil {
				return c.NoContent(http.StatusInternalServerError)
			}

			cmd.Env = append(cmd.Env, fmt.Sprintf("UPLOAD_%s=%s", strings.ToUpper(route.Upload), upload.Filename))
		}

		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}

		defer stdin.Close()

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}

		defer stdout.Close()

		err = cmd.Start()
		if err != nil {
			return c.NoContent(http.StatusInternalServerError)
		}

		payload, err := ioutil.ReadAll(c.Request().Body)
		if err != nil {
			return c.NoContent(http.StatusInternalServerError)
		}

		stdin.Write(payload)
		stdin.Close()

		output, err := ioutil.ReadAll(stdout)
		if err != nil {
			return c.NoContent(http.StatusInternalServerError)
		}

		if err = cmd.Wait(); err != nil {
			httpCode, err := strconv.Atoi(strings.TrimSpace(string(output)))
			if err == nil {
				return c.NoContent(httpCode)
			}

			return c.NoContent(http.StatusInternalServerError)
		}

		return c.String(http.StatusOK, string(output))
	}
}

func main() {
	data, err := ioutil.ReadFile("app.yml")
	if err != nil {
		panic(err)
	}

	app := Application{
		Listen: ":8088",
	}

	if err := yaml.Unmarshal(data, &app); err != nil {
		panic(err)
	}

	e := echo.New()

	e.Use(func(h echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("app", app)
			return h(c)
		}
	})

	for _, route := range app.Routes {
		switch route.Method {
		case "GET":
			e.GET(route.Path, handler(route))
		case "POST":
			e.POST(route.Path, handler(route))
		case "PUT":
			e.PUT(route.Path, handler(route))
		case "DELETE":
			e.DELETE(route.Path, handler(route))
		}
	}

	if err = e.Start(app.Listen); err != nil {
		panic(err)
	}
}
