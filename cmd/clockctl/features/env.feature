Feature: clockctl environment resolution
  Environment variables should resolve base URL, timeout, and auth settings predictably.

  Scenario: Resolve default server base URL when env is empty
    When I resolve the server base URL
    Then the resolved base URL should be "http://localhost:8080"

  Scenario: Resolve base URL from docker-style host
    Given I set env "CLOCK_SERVER_HOST" to "tcp://clock-host:8080"
    When I resolve the server base URL
    Then the resolved base URL should be "https://clock-host:8080"

  Scenario: Explicit base URL overrides host mapping
    Given I set env "CLOCK_SERVER_HOST" to "tcp://clock-host:8080"
    And I set env "CLOCK_SERVER_BASE_URL" to "https://override.example.com/"
    When I resolve the server base URL
    Then the resolved base URL should be "https://override.example.com"

  Scenario: API client reads env timeout and token
    Given I set env "CLOCK_SERVER_BASE_URL" to "https://clock.example.com"
    And I set env "CLOCK_SERVER_TOKEN" to "token-123"
    And I set env "CLOCKCTL_TIMEOUT_MS" to "1500"
    When I create an API client from environment
    Then the resolved base URL should be "https://clock.example.com"
    And the client token should be "token-123"
    And the client timeout should be 1500 milliseconds

  Scenario: parseTimeout returns fallback for invalid values
    Given I set CLOCKCTL_TIMEOUT_MS to "invalid"
    When I parse timeout using fallback 5000 milliseconds
    Then the parsed timeout should be 5000 milliseconds

  Scenario: parseBoolEnv uses fallback when env is missing
    When I parse bool env "CLOCKCTL_ALLOW_INSECURE_HTTP" with fallback "false"
    Then the parsed bool should be "false"

  Scenario: parseBoolEnv reads explicit true
    Given I set env "CLOCKCTL_ALLOW_INSECURE_HTTP" to "true"
    When I parse bool env "CLOCKCTL_ALLOW_INSECURE_HTTP" with fallback "false"
    Then the parsed bool should be "true"
