Feature: Clock server command APIs
  Command endpoints should validate payloads, enforce access scope, and dispatch valid commands.

  Scenario: Message command accepts valid payloads
    Given the API handler is running
    And I use bearer token "test-token"
    When I send a "POST" request to "/commands/messages" with JSON:
      """
      {"deviceId":"clock-1","message":"hello","durationSeconds":10}
      """
    Then the response status should be 202
    And the JSON response field "result" should equal "sent"
    And exactly 1 command should be dispatched
    And the last dispatched command type should be "display_message"
    And the last dispatched target device should be "clock-1"

  Scenario: Message command rejects invalid duration
    Given the API handler is running
    And I use bearer token "test-token"
    When I send a "POST" request to "/commands/messages" with JSON:
      """
      {"deviceId":"clock-1","message":"hello","durationSeconds":0}
      """
    Then the response status should be 400
    And the JSON response field "error" should equal "invalid command"
    And exactly 0 command should be dispatched

  Scenario: Message command rejects unknown JSON fields
    Given the API handler is running
    And I use bearer token "test-token"
    When I send a "POST" request to "/commands/messages" with JSON:
      """
      {"deviceId":"clock-1","message":"hello","durationSeconds":10,"unexpected":"value"}
      """
    Then the response status should be 400
    And the JSON response field "error" should contain "unknown field"
    And exactly 0 command should be dispatched

  Scenario: Alarm command accepts valid payloads
    Given the API handler is running
    And I use bearer token "test-token"
    When I send a "POST" request to "/commands/alarms" with JSON:
      """
      {"deviceId":"clock-1","alarmTime":"2099-01-01T07:00:00Z","label":"wake up"}
      """
    Then the response status should be 202
    And the JSON response field "result" should equal "scheduled"
    And exactly 1 command should be dispatched
    And the last dispatched command type should be "set_alarm"

  Scenario: Alarm command rejects non-RFC3339 timestamps
    Given the API handler is running
    And I use bearer token "test-token"
    When I send a "POST" request to "/commands/alarms" with JSON:
      """
      {"deviceId":"clock-1","alarmTime":"tomorrow at seven","label":"wake up"}
      """
    Then the response status should be 400
    And the JSON response field "error" should equal "alarmTime must be RFC3339"
    And exactly 0 command should be dispatched

  Scenario: Alarm command rejects past timestamps
    Given the API handler is running
    And I use bearer token "test-token"
    When I send a "POST" request to "/commands/alarms" with JSON:
      """
      {"deviceId":"clock-1","alarmTime":"2000-01-01T07:00:00Z","label":"wake up"}
      """
    Then the response status should be 400
    And the JSON response field "error" should equal "invalid command"
    And exactly 0 command should be dispatched

  Scenario: Brightness command accepts valid payloads
    Given the API handler is running
    And I use bearer token "test-token"
    When I send a "PUT" request to "/commands/brightness" with JSON:
      """
      {"deviceId":"clock-1","level":42}
      """
    Then the response status should be 202
    And the JSON response field "result" should equal "updated"
    And exactly 1 command should be dispatched
    And the last dispatched command type should be "set_brightness"

  Scenario: Brightness command rejects unsupported HTTP methods
    Given the API handler is running
    And I use bearer token "test-token"
    When I send a "POST" request to "/commands/brightness"
    Then the response status should be 405
    And exactly 0 command should be dispatched

  Scenario: Device scope blocks commands to unauthorized devices
    Given the API handler authorizes only device "clock-allowed"
    And I use bearer token "scoped-token"
    When I send a "POST" request to "/commands/messages" with JSON:
      """
      {"deviceId":"clock-denied","message":"hello","durationSeconds":10}
      """
    Then the response status should be 403
    And the JSON response field "error" should equal "forbidden for target device"
    And exactly 0 command should be dispatched

  Scenario: Downstream send failures return bad gateway
    Given the sender fails downstream
    And I use bearer token "test-token"
    When I send a "POST" request to "/commands/messages" with JSON:
      """
      {"deviceId":"clock-1","message":"hello","durationSeconds":10}
      """
    Then the response status should be 502
    And the JSON response field "error" should equal "command dispatch failed"
    And exactly 1 command should be dispatched

  Scenario: Request body size limit is enforced
    Given the API handler limits request bodies to 16 bytes
    And I use bearer token "test-token"
    When I send a "POST" request to "/commands/messages" with JSON:
      """
      {"deviceId":"clock-1","message":"hello","durationSeconds":10}
      """
    Then the response status should be 400
    And the JSON response field "error" should contain "request body too large"
    And exactly 0 command should be dispatched
