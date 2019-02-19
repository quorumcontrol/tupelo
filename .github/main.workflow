workflow "Docker Build & Push" {
  on = "push"
  resolves = [
    "Docker Push Ref Image",
    "Docker Tag Images",
  ]
}

action "Private Dep Ensure" {
  uses = "./.github/actions/dep"
  secrets = ["SSH_PRIVATE_KEY"]
}

action "Docker Login" {
  uses = "actions/docker/login@8cdf801b322af5f369e00d85e9cf3a7122f49108"
  secrets = ["DOCKER_PASSWORD", "DOCKER_USERNAME"]
  needs = ["Private Dep Ensure"]
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

action "Docker Push Release Image" {
  uses = "actions/docker/cli@8cdf801b322af5f369e00d85e9cf3a7122f49108"
  needs = ["Docker Tag Images"]
  args = "push quorumcontrol/tupelo:${GITHUB_REF}"
}

action "Docker Push SHA Image" {
  uses = "actions/docker/cli@8cdf801b322af5f369e00d85e9cf3a7122f49108"
  needs = ["Docker Tag Images"]
  args = "push quorumcontrol/tupelo:${IMAGE_SHA}"
}

action "Docker Push Ref Image" {
  uses = "actions/docker/cli@8cdf801b322af5f369e00d85e9cf3a7122f49108"
  needs = ["Docker Tag Images"]
  args = "push quorumcontrol/tupelo:${IMAGE_REF}"
}

workflow "Docker Tag Deletion" {
  on = "delete"
  resolves = ["Docker Delete Tag"]
}

action "Docker Delete Tag" {
  uses = "./.github/actions/docker-delete-tag"
  secrets = ["DOCKER_USERNAME", "DOCKER_PASSWORD"]
  args = "quorumcontrol/tupelo"
}
