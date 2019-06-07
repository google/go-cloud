# DO NOT REMOVE! The following line informs gocdk of the Docker image's name.
# gocdk-image: {{.ProjectName}}

# Step 1: Build Go binary.
FROM golang:1.12 as build
COPY . /src
WORKDIR /src
ENV GO111MODULE on
ENV GOPROXY https://proxy.golang.org/
RUN go build -o {{.ProjectName}} && go clean -modcache

# Step 2: Create image with built Go binary.
FROM gcr.io/distroless/base
COPY --from=build /src/{{.ProjectName}} /{{.ProjectName}}
ENV PORT 8080
EXPOSE 8080
ENTRYPOINT ["/{{.ProjectName}}"]
