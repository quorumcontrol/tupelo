workflow "Docker Build & Push" {
  on = "push"
  resolves = ["Docker Push Ref Image"]
}

workflow "Docker Tag Deletion" {
  on = "delete"
  resolves = ["Docker Delete Tag"]
}

workflow "Docker Build & Push Latest" {
  on = "release"
  resolves = ["Docker Push Latest Image"]
}

action "Private Go Mod" {
  uses = "./.github/actions/make"
  args = "vendor"
  secrets = ["SSH_PRIVATE_KEY"]
}

action "Prepare Build Environment" {
  uses = "./.github/actions/make"
  args = "github-prepare"
  needs = ["Private Go Mod"]
  secrets = ["SSH_PRIVATE_KEY"]
}

action "Docker Login" {
  uses = "actions/docker/login@8cdf801b322af5f369e00d85e9cf3a7122f49108"
  secrets = ["DOCKER_PASSWORD", "DOCKER_USERNAME"]
  needs = ["Prepare Build Environment"]
}

action "Docker Build Container" {
  uses = "actions/docker/cli@8cdf801b322af5f369e00d85e9cf3a7122f49108"
  args = "build -t imagebuild ."
  needs = ["Docker Login"]
}

action "Docker Tag Images" {
  uses = "actions/docker/tag@8cdf801b322af5f369e00d85e9cf3a7122f49108"
  args = "imagebuild quorumcontrol/tupelo"
  needs = ["Docker Build Container"]
}

action "Docker Push Ref Image" {
  uses = "actions/docker/cli@8cdf801b322af5f369e00d85e9cf3a7122f49108"
  needs = ["Docker Tag Images"]
  args = "push quorumcontrol/tupelo:${IMAGE_REF}"
}

action "On Latest Release" {
  uses = "./.github/actions/filters"
  args = "latest-release"
  needs = ["Docker Tag Images"]
  secrets = ["GITHUB_TOKEN"]
}

action "Docker Push Latest Image" {
  uses = "actions/docker/cli@8cdf801b322af5f369e00d85e9cf3a7122f49108"
  needs = ["On Latest Release"]
  args = "push quorumcontrol/tupelo:latest"
}

action "Docker Delete Tag" {
  uses = "./.github/actions/docker-delete-tag"
  secrets = ["DOCKER_USERNAME", "DOCKER_PASSWORD"]
  args = "quorumcontrol/tupelo"
}
