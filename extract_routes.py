import re

file_path = 'internal/server/server.go'

with open(file_path, 'r') as f:
    lines = f.readlines()

routes = []
current_prefix = ""

# Pattern for route methods
# r.Get("/path", handler)
# r.Post("/path", handler)
# r.With(...).Get("/path", handler)
# r.With(...).Patch("/path", handler)
# r.Route("/path", func(r chi.Router) { ... })
# r.Group(func(r chi.Router) { ... })

method_pattern = re.compile(r'r\.(\w+)\("([^"]+)"')
with_pattern = re.compile(r'r\.With\([^)]+\)\.(\w+)\("([^"]+)"')
route_pattern = re.compile(r'r\.Route\("([^"]+)"')
group_pattern = re.compile(r'r\.Group\(func\(r chi\.Router\) \{')

# For routing, we need to handle nesting. 
# We'll use a stack to keep track of the current prefix.

stack = []

for i, line in enumerate(lines):
    line = line.strip()
    
    # Check for Route
    route_match = route_pattern.search(line)
    if route_match:
        new_prefix = route_match.group(1)
        # If it's a route, it might be a group or just a route. 
        # In chi, r.Route starts a new router.
        # However, if it's followed by a func(r chi.Router) { it's a sub-router.
        # If it's not, it's usually just adding to the current router.
        # But chi.Route is used for grouping.
        
        # Let's see the next line or subsequent lines for the function body
        # Actually, for simplicity, let's assume r.Route and r.Group are for nesting.
        
        # We need to find the matching closing brace.
        # This is hard with regex. Let's use a simple approach.
        pass

    # We'll try a different approach.
    # Let's look for all method calls: .Get, .Post, .Put, .Patch, .Delete
    # and their preceding r. or r.With(...).
    pass

# Revised approach:
# 1. Find all lines that look like r.Get, r.Post, etc.
# 2. For each, determine its full path by looking at the current group/route stack.

# Since I don't have a full parser, I'll just search for the lines that define routes
# and manually check the context.

# Wait, I can just use grep/regex to find all lines with Get/Post/Put/Patch/Delete 
# and then try to reconstruct the path.

routes = []

# Let's try to find all lines that define a route
for i, line in enumerate(lines):
    line_stripped = line.strip()
    
    # Find method calls
    match = re.search(r'r\.(With\(.*?\)\.)?(Get|Post|Put|Patch|Delete|Route)\("([^"]+)"', line_stripped)
    if match:
        method_part = match.group(2)
        path_part = match.group(3)
        
        if method_part == "Route":
            # This is a grouping route. We need to find the scope.
            # Let's track the stack of paths.
            pass
        else:
            # This is a terminal route.
            pass

# Actually, let's just use the content of the file directly and try to identify the routes.
# I'll perform a manual extraction by looking at the segments.
