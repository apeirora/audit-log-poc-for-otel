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
WORKDIR /app
COPY --from=build /app/target/*.jar app.jar
EXPOSE 8080
ENTRYPOINT ["java", "-jar", "app.jar"]
