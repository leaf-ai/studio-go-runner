FROM golang:1.9rc2 as builder

WORKDIR /project/src/github.com/SentientTechnologies/studio-go-runner

COPY . /project/src/github.com/SentientTechnologies/studio-go-runner

ENV GOPATH /project

RUN ./build.sh
