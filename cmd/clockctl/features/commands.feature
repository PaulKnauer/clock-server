Feature: clockctl command payloads
  The CLI client should send the expected HTTP request shape for each command.

  Scenario: Message command sends POST payload
    Given the server base URL is "http://localhost:8080"
    When I send a message command for device "clock-1" message "hello" and duration 15
    Then the send call should succeed
    And the request method should be "POST"
    And the request path should be "/commands/messages"
    And the request JSON field "deviceId" should equal "clock-1"
    And the request JSON field "message" should equal "hello"
    And the request JSON field "durationSeconds" should equal "15"

  Scenario: Alarm command sends POST payload
    Given the server base URL is "http://localhost:8080"
    When I send an alarm command for device "clock-2" time "2099-01-01T07:00:00Z" label "wake"
    Then the send call should succeed
    And the request method should be "POST"
    And the request path should be "/commands/alarms"
    And the request JSON field "deviceId" should equal "clock-2"
    And the request JSON field "alarmTime" should equal "2099-01-01T07:00:00Z"
    And the request JSON field "label" should equal "wake"

  Scenario: Brightness command sends PUT payload
    Given the server base URL is "http://localhost:8080"
    When I send a brightness command for device "clock-3" level 77
    Then the send call should succeed
    And the request method should be "PUT"
    And the request path should be "/commands/brightness"
    And the request JSON field "deviceId" should equal "clock-3"
    And the request JSON field "level" should equal "77"

  Scenario: Non-2xx response returns status error
    Given the server base URL is "http://localhost:8080"
    And the server responds with status 502 and body:
      """
      {"error":"bad gateway"}
      """
    When I send a message command for device "clock-4" message "hello" and duration 10
    Then the send call should fail containing "status=502"

  Scenario: Transport errors are propagated
    Given the server base URL is "http://localhost:8080"
    And the transport fails with "dial tcp timeout"
    When I send a message command for device "clock-5" message "hello" and duration 10
    Then the send call should fail containing "call server"
