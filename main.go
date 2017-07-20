package main

import "fmt"
import "strings"
import "net/http"
import "io/ioutil"
import "os/exec"
import "strconv"
import "github.com/labstack/echo"
import yaml "gopkg.in/yaml.v2"

type Application struct {
  Routes []Route `yaml:"routes,omitempty"`
}

type Route struct {
  Path   string `yaml:"path,omitempty"`
  Method string `yaml:"method,omitempty"`
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

    for key, value := range c.Request().Header {
      cmd.Env = append(cmd.Env, fmt.Sprintf("HEADER_%s=%s", strings.ToUpper(key), value))
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

  app := Application{}
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
    case "DELETE":
      e.DELETE(route.Path, handler(route))
    }
  }

  e.Start(":8080")
}
