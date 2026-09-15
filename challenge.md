Q3: Feature Flag Question
Coding Interview: Feature Flag REST API Service

Mandatory Environment Rules
To ensure your work is saved and evaluated correctly, you must adhere to the following operational constraints:

Stay in the Workspace: All development must occur inside the /workspaces directory within your IDE.
No Host Saving: Do not save code or assets to the host machine’s Desktop or Downloads folder. These areas are not bridged to the container; data saved there will be permanently lost when the session ends.
Final Submission: You must push your code and architecture diagram to a personal GitHub repository before the timer expires.
Session Cleanup: Sign out of all personal accounts (GitHub, Cursor, Browser) before handing back the workstation.
Logistics and Tooling
Item

Details

Time Limit

3 Hours

Supported Languages

Go, Python, Java, or TypeScript / Node.js.

(Standard extensions for YAML, Python, Go, and Java are highly recommended).

IDE Environment

VS Code or Cursor, bridged directly to a Ubuntu 24 Dockerized container.

AI Assistance

GitHub Copilot, Claude Code, and Cursor are permitted.

You are encouraged to use Chrome, documentation, and any AI tools to assist your development. You must sign in using your personal accounts and remain fully responsible for the review, architecture, and correctness of all generated outputs.

Permissions

You have passwordless sudo access to install additional dependencies via apt or brew.

Pre-Installed Utilities

GitHub CLI (gh), doctl, s3cmd, jq, yq, neovim, and core image processing libraries.

Project Objective
Summary: Build a production-ready REST API service that stores feature flags, manages flag states globally and per-user, and evaluates feature availability for specific users while utilizing caching for performance.

Functional Expectations
At a minimum, your service should demonstrate:

Creation & Storage: Allow the creation of feature flags (e.g., name, description, default state) and store these configurations persistently.
Management: Allow enabling or disabling a feature flag globally (for all users) or for a specific user.
Evaluation & Performance: Provide an endpoint to evaluate whether a feature is enabled for a given user. Implement caching for evaluations or flag data to improve performance.
Standards: Return appropriate and sensible HTTP status codes for all operations.
Engineering Expectations
Your solution should reflect what you believe constitutes a production-ready service. We require:

Architecture Flow Diagram: Include a diagram in your repository mapping the request lifecycle and data flow at a high level. This will serve as the anchor for your technical review.
Validation: Sensible error handling, input validation, and edge-case management.
Testing: Unit or integration tests that demonstrate correctness.
CI/CD: A basic pipeline configuration (e.g., GitHub Actions).
Documentation: A well-organized codebase and a README providing clear setup, execution, and testing instructions.
Extensions & Next Steps
If time permits, you are encouraged to expand on your solution:

Deployment: Deploy your service to DigitalOcean.
Customer-Centric Features: Add additional features you would expect a product like this to have, using your imagination and thinking from a customer's perspective.
Good luck. We look forward to reviewing your solution.