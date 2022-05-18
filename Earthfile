VERSION 0.6

FROM golang:1
WORKDIR /app

all:
  BUILD +docker

docker:
  FROM DOCKERFILE .

  ARG EARTHLY_GIT_PROJECT_NAME
  ARG EARTHLY_TARGET_TAG_DOCKER
  ARG IMAGE_TAG=${EARTHLY_TARGET_TAG_DOCKER}
  ARG IMAGE_NAME=ghcr.io/${EARTHLY_GIT_PROJECT_NAME}:${IMAGE_TAG}
  ARG CACHE_FROM=ghcr.io/${EARTHLY_GIT_PROJECT_NAME}:latest

  FOR name IN ${IMAGE_NAME}
    SAVE IMAGE --cache-from="${CACHE_FROM}" --push ${name}
  END

golangci-lint:
  FROM golangci/golangci-lint:latest
  WORKDIR /app
  COPY entrypoint .
  ARG CI
  IF [ "${CI}" = "true" ]
    RUN golangci-lint run --path-prefix=entrypoint --out-format=line-number
  ELSE
    RUN golangci-lint --color=always run --path-prefix=entrypoint
  END

test:
  BUILD +golangci-lint
  BUILD +yamllint

yamllint:
  FROM alpine:3.15
  RUN apk --quiet --no-progress --no-cache add yamllint

  COPY . .

  ARG CI
  IF [ "${CI}" = "true" ]
    RUN yamllint --format=parsable .
  ELSE
    RUN yamllint --format=colored .
  END
