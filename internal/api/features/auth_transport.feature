Feature: Authentication and transport security
  The API should enforce bearer auth, auth-failure throttling, and TLS policy.

  Scenario: Command endpoint rejects missing bearer token
    Given the API handler is running
    And I clear the bearer token
    When I send a "POST" request to "/commands/messages" with JSON:
      """
      {"deviceId":"clock-1","message":"hello","durationSeconds":10}
      """
    Then the response status should be 401
    And the JSON response field "error" should equal "unauthorized"
    And exactly 0 command should be dispatched

  Scenario: Command endpoint rejects invalid bearer token
    Given the API handler is running
    And I use bearer token "wrong-token"
    When I send a "POST" request to "/commands/messages" with JSON:
      """
      {"deviceId":"clock-1","message":"hello","durationSeconds":10}
      """
    Then the response status should be 401
    And the JSON response field "error" should equal "unauthorized"
    And exactly 0 command should be dispatched

  Scenario: Auth failure rate limiter blocks repeated unauthorized calls
    Given the API handler rate limits auth failures to 1 per minute
    And I am calling from remote address "203.0.113.10:1234"
    And I clear the bearer token
    When I send a "GET" request to "/ready"
    Then the response status should be 401
    When I send a "GET" request to "/ready"
    Then the response status should be 429
    And the JSON response field "error" should equal "too many auth failures"

  Scenario: TLS is required when enabled
    Given the API handler requires TLS
    And I use bearer token "test-token"
    When I send a "GET" request to "/ready"
    Then the response status should be 426
    And the JSON response field "error" should equal "https required"

  Scenario: Proxy TLS headers are honored when configured
    Given the API handler requires TLS and trusts proxy headers
    And I use bearer token "test-token"
    And I set request header "X-Forwarded-Proto" to "https"
    When I send a "GET" request to "/ready"
    Then the response status should be 200
    And the JSON response field "status" should equal "ready"
