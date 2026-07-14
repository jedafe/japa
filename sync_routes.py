import re
import os
from collections import defaultdict

def extract_routes_final(file_path):
    with open(file_path, 'r') as f:
        content = f.read()
    
    routes = []
    stack = [] # List of (prefix, brace_level)
    
    # Use a simple brace counter to track nesting
    brace_level = 0
    
    # Find all route-related calls
    # We look for r.Route, r.Group, and the HTTP methods.
    # Since we need to process them in order, we'll find all matches and sort them by position.
    
    pattern = re.compile(
        r'(?P<route>r\.Route\s*\(\s*["\'](?P<route_prefix>[^"\']+)["\']\s*,)|'
        r'(?P<group>r\.Group\s*\()|'
        r'(?P<method>\.(Get|Post|Put|Patch|Delete)\s*\(\s*["\'](?P<path>[^"\']+)["\'])'
    )
    
    # We also need to track braces to know when to pop the stack
    # We'll iterate through the content character by character or use finditer and handle braces in between.
    
    last_pos = 0
    for match in pattern.finditer(content):
        # Process braces between last match and current match
        inter_content = content[last_pos:match.start()]
        brace_level += inter_content.count('{')
        brace_level -= inter_content.count('}')
        
        # Pop stack based on brace level before processing current match
        # This is slightly off because the match itself might contain braces.
        # But the stack entries are pushed when the Route/Group is called,
        # and the block actually starts at the next '{'.
        
        if match.group('route'):
            prefix = match.group('route_prefix')
            # We push the prefix. We'll need to know at what brace level it was pushed.
            # The block starts at the first '{' after the match.
            start_brace_pos = content.find('{', match.end())
            # We count braces up to that start_brace_pos
            inter_content_to_brace = content[match.end():start_brace_pos] if start_brace_pos != -1 else ""
            current_level = brace_level + inter_content_to_brace.count('{') - inter_content_to_brace.count('}')
            stack.append((prefix, current_level))
            
        elif match.group('group'):
            # Group doesn't add a prefix, but it's a block.
            start_brace_pos = content.find('{', match.end())
            inter_content_to_brace = content[match.end():start_brace_pos] if start_brace_pos != -1 else ""
            current_level = brace_level + inter_content_to_brace.count('{') - inter_content_to_brace.count('}')
            stack.append(("", current_level))
            
        elif match.group('method'):
            method_name = match.group(2).upper()
            path = match.group('path')
            
            # Build full path from stack
            # Only include prefixes from levels that are still open (level >= current_level)
            # But we don't know current_level here. 
            # Let's use the current brace_level.
            
            full_path = ""
            # Since we don't have a perfect way to pop, let's just use the stack 
            # and pop when we see '}' in the inter_content.
            # Wait, the inter_content already updated brace_level.
            # We should pop from the stack any entry whose level is >= current brace_level.
            
            # Actually, a much simpler way:
            # Just use the stack as it is and pop whenever we see '}'
            # But we need to pop only if the stack is not empty.
            
            # Let's try this:
            # Whenever we see '}', if the stack is not empty, pop.
            # This assumes every r.Route/r.Group has exactly one closing '}'.
            
            # Let's refine the loop.
            pass
        
        last_pos = match.end()

    return routes

# Let's try a completely different approach:
# 1. Find all r.Route and r.Group calls.
# 2. For each, find its matching closing brace.
# 3. This creates a tree of prefixes.
# 4. Then find all .Get/.Post etc and see which Route/Group they are inside.

def extract_routes_tree(file_path):
    with open(file_path, 'r') as f:
        content = f.read()
        
    routes = []
    # Find all route/group starts
    starts = []
    for m in re.finditer(r'r\.(Route|Group)\s*\(', content):
        # Get the prefix if it's a Route
        prefix = ""
        if m.group(1) == 'Route':
            # Find the first string literal after the (
            prefix_match = re.search(r'["\']([^"\']+)["\']', content[m.end():])
            if prefix_match:
                prefix = prefix_match.group(1)
        
        # Find the start of the block {
        block_start = content.find('{', m.end())
        if block_start == -1: continue
        
        # Find the matching closing brace
        depth = 1
        block_end = -1
        for i in range(block_start + 1, len(content)):
            if content[i] == '{': depth += 1
            elif content[i] == '}': depth -= 1
            if depth == 0:
                block_end = i
                break
        
        if block_end != -1:
            starts.append({
                'prefix': prefix,
                'start': block_start,
                'end': block_end,
                'full_match': content[m.start():block_end+1]
            })
            
    # Now find all HTTP methods
    method_regex = re.compile(r'\.(Get|Post|Put|Patch|Delete)\s*\(\s*["\']([^"\']+)["\']')
    for m in method_regex.finditer(content):
        method = m.group(1).upper()
        path = m.group(2)
        pos = m.start()
        
        # Find all Route/Group blocks that enclose this position
        enclosing_prefixes = []
        # To maintain order, we should find them and sort by start position
        for s in starts:
            if s['start'] < pos < s['end']:
                enclosing_prefixes.append((s['start'], s['prefix']))
        
        enclosing_prefixes.sort()
        full_path = "".join([p for start, p in enclosing_prefixes]) + path
        full_path = full_path.replace('//', '/')
        if not full_path.startswith('/'):
            full_path = '/' + full_path
            
        routes.append((method, full_path))
        
    return routes

def parse_catalog(file_path):
    with open(file_path, 'r') as f:
        lines = f.readlines()
    
    groups = defaultdict(list)
    current_group = "Uncategorized"
    in_block = False
    
    for line in lines:
        line = line.strip()
        if not line: continue
        if line.startswith('## '):
            current_group = line[3:].strip()
            in_block = False
            continue
        if line.startswith('```'):
            in_block = not in_block
            continue
        if in_block:
            parts = line.split()
            if len(parts) >= 2:
                method = parts[0].upper()
                path = parts[1]
                groups[current_group].append((method, path))
    return groups

def synchronize():
    go_routes = extract_routes_tree('internal/server/server.go')
    print(f"Extracted {len(go_routes)} routes from Go.")
    
    catalog_groups = parse_catalog('.jeos/conventions/api-routes-catalog.md')
    code_routes_set = set(go_routes)
    
    group_order = []
    with open('.jeos/conventions/api-routes-catalog.md', 'r') as f:
        for line in f:
            if line.startswith('## '):
                group_name = line[3:].strip()
                if group_name not in group_order:
                    group_order.append(group_name)

    updated_groups = {}
    assigned_code_routes = set()
    phantom = []
    
    for group in group_order:
        group_routes = catalog_groups.get(group, [])
        new_group_routes = []
        
        for m, p in group_routes:
            norm_p = p
            if group != "Infrastructure (root paths — no /api/v1/ prefix)" and not p.startswith('/api/v1'):
                norm_p = '/api/v1' + p if p.startswith('/') else '/api/v1/' + p
            
            if (m, norm_p) in code_routes_set:
                new_group_routes.append((m, p))
                assigned_code_routes.add((m, norm_p))
            else:
                phantom.append((m, p, group))
        
        updated_groups[group] = new_group_routes

    undocumented = [r for r in go_routes if r not in assigned_code_routes]
    
    if undocumented:
        undoc_group = "Newly Discovered (Undocumented)"
        group_order.append(undoc_group)
        updated_groups[undoc_group] = undocumented

    with open('.jeos/conventions/api-routes-catalog.md', 'w') as f:
        for group in group_order:
            f.write(f"## {group}\n")
            f.write("```\n")
            routes = updated_groups.get(group, [])
            for m, p in routes:
                f.write(f"{m:<6} {p}\n")
            f.write("```\n\n")
            
    print(f"Added {len(undocumented)} undocumented routes.")
    print(f"Removed {len(phantom)} phantom routes.")

if __name__ == '__main__':
    synchronize()
