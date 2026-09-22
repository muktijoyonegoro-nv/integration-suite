# Containerfile for sort-service runtime
FROM eclipse-temurin:11-jre-jammy

RUN apt-get update && apt-get install -y --no-install-recommends bash curl && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# The staged Play Framework build from `sbt stage`
COPY target/universal/stage /app

EXPOSE 9000

ENTRYPOINT ["/app/bin/sort"]
CMD ["-Dplay.server.pidfile.path=/dev/null"]
