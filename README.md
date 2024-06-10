# RSS Aggregator with GO

[![Go](https://img.shields.io/badge/Go-1.22-blue)](https://golang.org/)
[![Postgres](https://img.shields.io/badge/Postgres-16.3-blue)](https://www.postgresql.org/)
[![Angular](https://img.shields.io/badge/Angular-12.2-red)](https://angular.io/)
[![Redis](https://img.shields.io/badge/Redis-6.2-red)](https://redis.io/)

The project is an RSS Aggregator built with Go, a statically typed, compiled programming language developed by Google. An RSS Aggregator is a type of software that fetches and consolidates RSS feeds from various sources into a single location. RSS, which stands for Really Simple Syndication, is a web feed that allows users and applications to access updates to websites in a standardized, computer-readable format.

This particular RSS Aggregator is designed to fetch data from various RSS feeds and aggregate it. The aggregated data is then stored in a Postgres database for later retrieval and analysis. Postgres, or PostgreSQL, is a powerful, open-source object-relational database system.

The requirements section lists the technologies needed to run the project. In this case, the project requires Go (specifically version 1.22) and Postgres (specifically version 16.3). These versions are likely the ones the project was developed and tested with, so using them would help ensure compatibility and smooth operation.

## Next Steps

This project starts to extend the tutorial / crash course [Golang Web Server and RSS Scraper | Full Tutorial](https://www.youtube.com/watch?v=dpXhDzgUSe4). My goal is to create a complete RSS aggregator to aggregate the rss feeds from [**fitgirl-repacks**](https://fitgirl-repacks.site/), [**DODI-repacks**](https://dodi-repacks.site/), and [**Nyaa**](https://nyaa.si/) and retrieve data from SteamDB and AnimeDB to get the latest game and anime releases and help users find the latest content without having to visit multiple websites.

<p align="center">
    <img src="https://media1.giphy.com/media/v1.Y2lkPTc5MGI3NjExOWF0MDNscTk2am1ubzNsYTZzZWpndzE0ODR5bWVzb2c3Z3F3Nm8xdiZlcD12MV9pbnRlcm5hbF9naWZfYnlfaWQmY3Q9Zw/gIGomV9gjk9R3l66mQ/giphy.webp" alt="pirate alt" width="300px">
</p>

## TODO

- [x] Create a Go program to fetch RSS feeds
- [x] Parse the fetched data and store it in a Postgres database
- [x] Implement a REST API to retrieve the aggregated data
- [ ] Refactor the code to improve readability and maintainability using the project [go-blueprint](https://github.com/Melkeydev/go-blueprint)
- [ ] Write tests to ensure the program works as expected
- [ ] Define a front-end interface to display the aggregated data (Angular or Next.js)
- [ ] Add authentication and authorization to the REST API
- [ ] Implement caching to improve performance (Redis?) (optional)
- [ ] Add error handling and logging to the program (optional)
- [ ] Refactor the code to improve readability and maintainability (optional)
- [ ] Document the project to help other developers understand and contribute (optional)
- [ ] Optimize the program for scalability and efficiency (optional)
- [ ] Add support for additional RSS feed formats and sources (optional)
- [ ] Integrate with third-party services for additional functionality (optional)
- [ ] Implement a CI/CD pipeline to automate testing and deployment
- [ ] Deploy the project to a server for public access
- [ ] Monitor the project for performance and stability (optional)
