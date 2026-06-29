Web development bugs can be broadly categorized into functional flaws, visual or layout issues, performance bottlenecks, and security vulnerabilities. Each type impacts the user experience and the system's overall integrity differently.

## 1. Functional Bugs
These occur when a feature fails to behave according to business requirements.

* Form & Validation Errors: Submission buttons fail, data doesn't save, or error messages do not trigger for invalid input.
* Navigation & Routing: Broken links lead to a 404 page, or the "back" button breaks the application state.
* Calculation Errors: E-commerce carts miscalculate taxes, shipping fees, or total costs.

## 2. UI/UX (Visual & Usability) Bugs
These relate to the frontend and user interface where elements misbehave visually or hinder navigation.

* Responsive Design Flaws: Text overlaps or gets cut off when a site is viewed on smaller mobile screens.
* Z-Index / Overlay Issues: Modal popups or dropdown menus cannot be clicked because they are hidden behind another element, or they block interactions.
* Broken Assets: Missing images, icons, or flash-of-unstyled-text (FOUT) caused by unlinked files or network latency.

## 3. Integration & API Bugs
These happen when different parts of your system—like the frontend, backend, or third-party services—fail to communicate.

* Failed API Calls: The frontend sends a request but receives no response, an empty payload, or a 500 Internal Server Error.
* Sync Issues: Payment gateways take a customer's money but fail to update the order status in the inventory system.

## 4. Performance Bugs
These prevent the website from running smoothly or efficiently.

* High Latency: Pages take too long to load (e.g., >3 seconds), often due to massive, uncompressed images or unoptimized database queries.
* Memory Leaks: The website becomes sluggish over time or freezes because JavaScript processes continuously consume memory without clearing it.

## 5. Security Bugs
These are dangerous vulnerabilities that allow malicious actors to access the system.

* SQL Injection (SQLi): Unsanitized input fields allow users to input harmful database commands.
* Cross-Site Scripting (XSS): The application serves unverified, malicious scripts to other users' browsers.

## 6. Syntax & Logic Errors
These are coding errors stemming from mistakes made while writing the program.

* Syntax Errors: Missing brackets, incorrect tokens, or unclosed quotes that cause the compiler to halt execution.
* Logic Errors: The code runs without crashing, but a flaw in the algorithm produces incorrect results (e.g., applying a 10% discount twice due to a misapplied conditional loop).
