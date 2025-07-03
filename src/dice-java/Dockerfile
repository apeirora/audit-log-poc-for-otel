# --- Build Stage ---
FROM sapmachine:24-jdk-headless-ubuntu AS build
# install maven
RUN apt-get update && apt-get -y install maven
# build the application
WORKDIR /app
# let's create separate layers for dependencies and source code
COPY pom.xml .
RUN mvn clean dependency:go-offline
COPY src ./src
RUN mvn package -DskipTests

# --- Run Stage ---
FROM sapmachine:24-jre-alpine

ARG REST_PORT=8082
ENV REST_PORT=${REST_PORT}

LABEL org.opencontainers.image.source=https://github.com/apeirora/audit-log-poc-for-otel

WORKDIR /app
COPY --from=build /app/target/*.jar app.jar

EXPOSE ${REST_PORT}
ENTRYPOINT ["java", "-jar", "app.jar"]
