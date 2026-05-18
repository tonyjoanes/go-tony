FROM mcr.microsoft.com/dotnet/sdk:8.0 AS build
WORKDIR /src
COPY src/MyApi/MyApi.csproj src/MyApi/
RUN dotnet restore src/MyApi/MyApi.csproj
COPY src/MyApi/ src/MyApi/
RUN dotnet publish src/MyApi/MyApi.csproj -c Release -o /app/publish

FROM mcr.microsoft.com/dotnet/aspnet:8.0
WORKDIR /app
COPY --from=build /app/publish .
EXPOSE 8080
ENTRYPOINT ["dotnet", "MyApi.dll"]
