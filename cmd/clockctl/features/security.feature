Feature: clockctl token transport safety
  The client should protect bearer tokens on insecure transports.

  Scenario: Token is attached over HTTPS
    Given the server base URL is "https://clock.example.com"
    And the bearer token is "secret-token"
    When I send a message command for device "clock-1" message "secure" and duration 10
    Then the send call should succeed
    And the Authorization header should be "Bearer secret-token"

  Scenario: Token is attached over local HTTP
    Given the server base URL is "http://localhost:8080"
    And the bearer token is "local-token"
    When I send a message command for device "clock-1" message "local" and duration 10
    Then the send call should succeed
    And the Authorization header should be "Bearer local-token"

  Scenario: Token is rejected over non-local HTTP by default
    Given the server base URL is "http://clock-server.local:8080"
    And the bearer token is "unsafe-token"
    When I send a message command for device "clock-1" message "blocked" and duration 10
    Then the send call should fail containing "refusing to send bearer token"

  Scenario: Insecure HTTP can be explicitly allowed
    Given the server base URL is "http://clock-server.local:8080"
    And the bearer token is "override-token"
    And insecure HTTP transport is allowed
    When I send a message command for device "clock-1" message "override" and duration 10
    Then the send call should succeed
    And the Authorization header should be "Bearer override-token"
