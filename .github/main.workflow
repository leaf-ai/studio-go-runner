workflow "Test " {
  on = "push"
  resolves = ["GitHub Action for Docker"]
}

action "Docker Registry" {
  uses = "actions/docker/login@master"
  secrets = ["DOCKER_USERNAME", "DOCKER_PASSWORD"]
}

action "Stencil Dockerfile" {
  uses = "docker://karlmutch/duat-stencil"
  needs = ["Docker Registry"]
  env = {
    DOCKERFILE = "Dockerfile_k8s_local"
  }
}

action "build" {
  uses = "actions/docker/cli@master"
  needs = ["Stencil Dockerfile"]
  args = "build -t leafai/studio-go-runner-build_k8s_local -f Dockerfile.stencil ."
}

action "GitHub Action for Docker" {
  uses = "actions/docker/cli@master"
  needs = ["build"]
  args = "push leafai/studio-go-runner-build_k8s_local"
}
