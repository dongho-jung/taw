---
description: Auto-detect and run project tests
---

# Run Tests

Automatically detects the project type and runs appropriate test commands.

## Step 1: Detect Project Type

Check for project configuration files:

```bash
ls -la package.json Cargo.toml pyproject.toml setup.py Makefile go.mod build.gradle pom.xml Gemfile 2>/dev/null || echo "NO_CONFIG"
```

## Step 2: Identify Test Command

### Node.js (package.json)
```bash
cat package.json 2>/dev/null | grep -E '"test"' && echo "NODE_PROJECT"
```
Command: `npm test` or `npm run test`

### Rust (Cargo.toml)
```bash
[ -f Cargo.toml ] && echo "RUST_PROJECT"
```
Command: `cargo test`

### Python (pyproject.toml or setup.py)
```bash
([ -f pyproject.toml ] || [ -f setup.py ]) && echo "PYTHON_PROJECT"
```
Commands (try in order):
1. `pytest` (if pytest installed)
2. `python -m pytest`
3. `python -m unittest discover`

### Go (go.mod)
```bash
[ -f go.mod ] && echo "GO_PROJECT"
```
Command: `go test ./...`

### Make (Makefile)
```bash
grep -E '^test:' Makefile 2>/dev/null && echo "MAKEFILE_TEST"
```
Command: `make test`

### Java/Gradle (build.gradle)
```bash
[ -f build.gradle ] && echo "GRADLE_PROJECT"
```
Command: `./gradlew test` or `gradle test`

### Java/Maven (pom.xml)
```bash
[ -f pom.xml ] && echo "MAVEN_PROJECT"
```
Command: `mvn test`

### Ruby (Gemfile)
```bash
[ -f Gemfile ] && echo "RUBY_PROJECT"
```
Commands: `bundle exec rspec` or `bundle exec rake test`

## Step 3: Run Tests

Execute the detected test command and capture output.

```bash
{TEST_COMMAND}
```

## Step 4: Report Results

Parse and summarize test results:
- Total tests
- Passed / Failed / Skipped
- Failed test names (if any)
- Duration

## Step 5: Log Result

```bash
echo "Tests: {PASS_COUNT} passed, {FAIL_COUNT} failed" >> $TAW_DIR/agents/$TASK_NAME/log
```

## Handling Failures

If tests fail:
1. List failed tests with error messages
2. Suggest fixes if patterns are obvious
3. Ask user: "Fix failing tests and retry?"

Proceed with test detection and execution.
