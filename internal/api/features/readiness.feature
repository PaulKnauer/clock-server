Feature: Health and readiness endpoints
  Operational endpoints should report service state and expose request metadata consistently.

  Scenario: Health endpoint is publicly accessible
    Given the API handler is running
    When I send a "GET" request to "/health"
    Then the response status should be 200
    And the JSON response field "status" should equal "ok"

  Scenario: Readiness endpoint requires authentication by default
    Given the API handler is running
    And I clear the bearer token
    When I send a "GET" request to "/ready"
    Then the response status should be 401

  Scenario: Readiness endpoint allows authenticated checks
    Given the API handler is running
    And I use bearer token "test-token"
    When I send a "GET" request to "/ready"
    Then the response status should be 200
    And the JSON response field "status" should equal "ready"

  Scenario: Readiness endpoint can allow unauthenticated checks
    Given the API handler allows unauthenticated readiness checks
    When I send a "GET" request to "/ready"
    Then the response status should be 200
    And the JSON response field "status" should equal "ready"

  Scenario: Readiness endpoint reports dependency failures
    Given the API handler has a failing readiness checker
    And I use bearer token "test-token"
    When I send a "GET" request to "/ready"
    Then the response status should be 503
    And the JSON response field "status" should equal "not_ready"

  Scenario: Readiness endpoint rejects unsupported methods
    Given the API handler is running
    And I use bearer token "test-token"
    When I send a "POST" request to "/ready"
    Then the response status should be 405

  Scenario: Response request IDs are generated when absent
    Given the API handler is running
    When I send a "GET" request to "/health"
    Then the response header "X-Request-Id" should have prefix "req-"

  Scenario: Response request IDs preserve caller supplied values
    Given the API handler is running
    And I set request header "X-Request-Id" to "trace-123"
    When I send a "GET" request to "/health"
    Then the response header "X-Request-Id" should equal "trace-123"
