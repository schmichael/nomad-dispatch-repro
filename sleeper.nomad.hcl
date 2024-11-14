job "sleeper" {
  type = "batch"

  parameterized {
    meta_required = ["dur"]
  }

  group "g" {

    task "sleeper" {
      driver = "docker"

      config {
        image   = "alpine:3"
        command = "/bin/sleep"
        args = ["${NOMAD_META_dur}"]
        auth_soft_fail = true
      }

      resources {
        cpu    = 100
        memory = 100
      }
    }
  }
}
