import re

def extract_routes(file_path):
    with open(file_path, 'r') as f:
        lines = f.readlines()

    routes = []
    stack = [] # List of (prefix, type) where type is 'route' or 'group'

    # Simple state machine to handle nesting
    # We'll look for r.Route, r.Group, and method calls.
    
    for i, line in enumerate(lines):
        line = line.strip()
        
        # Match r.Route("/path", ...) or r.Group(func(r chi.Router) {
        # Note: chi.Route and chi.Group both create a new router scope.
        
        route_match = re.search(r'r\.(Route|Group)\("([^"]+)"', line)
        if route_match:
            new_prefix = route_match.group(2)
            if not new_prefix.startswith('/'):
                new_prefix = '/' + new_prefix
            
            # If current prefix is /api/v1, and we add /auth, it becomes /api/v1/auth
            if stack:
                parent_prefix = stack[-1][0]
                if parent_prefix.endswith('/'):
                    new_prefix = parent_prefix + new_prefix.lstrip('/')
                else:
                    new_prefix = parent_prefix + '/' + new_prefix.lstrip('/')
            else:
                new_prefix = '/' + new_prefix.lstrip('/')

            stack.append((new_prefix, route_match.group(1)))
            continue

        # Match method calls: r.Get, r.Post, r.Put, r.Patch, r.Delete
        # Also handle r.With(...).Get(...)
        method_match = re.search(r'r\.(?:With\([^)]+\)\.)?(Get|Post|Put|Patch|Delete)\("([^"]+)"', line)
        if method_match:
            method = method_match.group(1)
            path = method_match.group(2)
            
            full_path = path
            if stack:
                parent_prefix = stack[-1][0]
                if parent_prefix.endswith('/'):
                    full_path = parent_prefix + path.lstrip('/')
                else:
                    full_path = parent_prefix + '/' + path.lstrip('/')
            
            # Ensure the path starts with /
            if not full_path.startswith('/'):
                full_path = '/' + full_path
                
            routes.append((method.upper(), full_path))
            continue

        # We need to handle the closing of the scope.
        # Since it's a Go file, it's usually a closing brace '}'.
        # But 'r.Route' and 'r.Group' use functions.
        # We'll track braces.
        if 'func(r chi.Router) {' in line or 'r.Group(func(r chi.Router) {' in line or 'r.Route("/' in line:
            # This is tricky. Let's just count braces.
            # A better way is to count indentation or use a real Go parser.
            # But for this task, let's try counting braces in the block.
            pass

    return routes

# Actually, I'll just manually find all the routes from the file content 
# because I have it in front of me and it's not THAT many.
# The regex approach is failing due to the complexity of Go code.

# I will use a different approach: I'll use grep to find all lines with methods 
# and then I will carefully reconstruct them.
