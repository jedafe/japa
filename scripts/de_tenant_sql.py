#!/usr/bin/env python3.13
"""Transform a sqlc query file to remove tenant_id references.

This is a mechanical helper for the multi-tenancy removal. It is run once
per subsystem; the result is always reviewed and tested with `make sqlc-gen`
before committing.

Limitations:
- Assumes each query body ends with the first `;` outside of `--` comments.
- Named parameters use the sqlc `@name` style.
- Positional parameters `$N` are renumbered after tenant parameters are removed.
"""
import re
import sys


def split_queries(text):
    """Split file into (comment+name_header, leading_comments, sql, trailing) pieces."""
    header_pattern = re.compile(r'^-- name: .*?$', re.MULTILINE)
    matches = list(header_pattern.finditer(text))
    result = []
    for i, m in enumerate(matches):
        header = m.group(0)
        start = m.end()
        end = matches[i + 1].start() if i + 1 < len(matches) else len(text)
        body = text[start:end]

        # Extract the first SQL statement (up to first `;` outside `--` comments)
        sql, sql_end = extract_first_statement(body)
        leading = body[:sql_end - len(sql)]
        trailing = body[sql_end:]
        result.append((header, leading, sql, trailing))
    leading_text = text[:matches[0].start()] if matches else text
    return result, leading_text


def extract_first_statement(text):
    """Return the first SQL statement and the index right after its `;`."""
    in_string = False
    string_char = None
    in_comment = False
    for i, ch in enumerate(text):
        if in_comment:
            if ch == '\n':
                in_comment = False
            continue
        if in_string:
            if ch == string_char:
                if i + 1 < len(text) and text[i + 1] == string_char:
                    # escaped quote in PostgreSQL string literal style
                    pass
                else:
                    in_string = False
                    string_char = None
            continue
        if ch == '-' and i + 1 < len(text) and text[i + 1] == '-':
            in_comment = True
            continue
        if ch == "'" or ch == '"':
            in_string = True
            string_char = ch
            continue
        if ch == ';' and not in_string and not in_comment:
            return text[:i + 1], i + 1
    return text, len(text)


def clean_whitespace(sql):
    # Collapse multiple spaces/tabs, keep newlines.
    sql = re.sub(r'[ \t]+', ' ', sql)
    # Remove trailing whitespace per line.
    sql = re.sub(r' +\n', '\n', sql)
    # Remove `WHERE ` empty conditions.
    sql = re.sub(r'\bWHERE\s+(?=[;\n])', '', sql, flags=re.IGNORECASE)
    sql = re.sub(r'\bWHERE\s+AND\s+', 'WHERE ', sql, flags=re.IGNORECASE)
    # Remove `ON ` empty conditions.
    sql = re.sub(r'\bON\s+(?=[;\n])', '', sql, flags=re.IGNORECASE)
    sql = re.sub(r'\bON\s+TRUE\s+AND\s+', 'ON ', sql, flags=re.IGNORECASE)
    # Trim lines.
    return '\n'.join(line.rstrip() for line in sql.split('\n'))


def remove_tenant_from_lists(sql):
    """Remove tenant_id from comma-separated column lists.

    Patterns like `id, tenant_id, email` or `u.tenant_id, u.id`.
    Does NOT remove inside named parameters (e.g. `@tenant_id`).
    """
    # Negative lookbehind for @ or : to avoid touching named parameters.
    sql = re.sub(r'(?<![@:])(?:(?:\w+)\.)?tenant_id\s*,\s*', '', sql, flags=re.IGNORECASE)
    sql = re.sub(r',\s*(?:(?:\w+)\.)?tenant_id\b', '', sql, flags=re.IGNORECASE)
    return sql


def remove_tenant_conditions(sql):
    """Remove tenant_id equality conditions from WHERE, JOIN, ON, etc."""
    value_re = r"(?:\$\d+|@\w+|'[^']*'|\"[^\"]*\")"
    comp = r'(?:(?:\w+)\.)?tenant_id\s*=\s*' + value_re
    joined = r'(?:(?:\w+)\.)?tenant_id\s*=\s*(?:(?:\w+)\.)?tenant_id'

    def remove_pattern(sql, cond):
        # As a standalone WHERE condition with optional following AND.
        sql = re.sub(r'\bWHERE\s+' + cond + r'(?:\s+AND\s+)?', 'WHERE ', sql, flags=re.IGNORECASE)
        sql = re.sub(r'\bWHERE\s+' + cond + r'(?=\s*[;\n])', '', sql, flags=re.IGNORECASE)
        # As a standalone ON condition with optional following AND.
        sql = re.sub(r'\bON\s+' + cond + r'(?:\s+AND\s+)?', 'ON ', sql, flags=re.IGNORECASE)
        sql = re.sub(r'\bON\s+' + cond + r'(?=\s*[;\n])', '', sql, flags=re.IGNORECASE)
        # Preceded by AND/OR.
        sql = re.sub(r'\s+(?:AND|OR)\s+' + cond, '', sql, flags=re.IGNORECASE)
        return sql

    sql = remove_pattern(sql, comp)
    sql = remove_pattern(sql, joined)

    # WHERE / ON empty cleanup.
    sql = re.sub(r'\bWHERE\s+(?=[;\n])', '', sql, flags=re.IGNORECASE)
    sql = re.sub(r'\bWHERE\s+\)', ') ', sql, flags=re.IGNORECASE)
    return sql


def remove_tenant_from_on_conflict(sql):
    """Remove tenant_id from ON CONFLICT (...) target."""
    sql = re.sub(r'ON\s+CONFLICT\s*\(\s*tenant_id\s*,\s*', 'ON CONFLICT (', sql, flags=re.IGNORECASE)
    sql = re.sub(r',\s*tenant_id\s*\)', ')', sql, flags=re.IGNORECASE)
    return sql


def parse_parens(text, start_idx):
    """Return the substring inside balanced parentheses starting at start_idx."""
    assert text[start_idx] == '('
    depth = 1
    i = start_idx + 1
    while i < len(text) and depth > 0:
        if text[i] == '(':
            depth += 1
        elif text[i] == ')':
            depth -= 1
        i += 1
    return text[start_idx + 1:i - 1], text[i:]


def _transform_one_insert(insert_sql):
    """Transform a single INSERT statement that starts with INSERT INTO."""
    pattern = re.compile(r'INSERT\s+INTO\s+(\w+)\s*', re.IGNORECASE | re.DOTALL)
    m = pattern.match(insert_sql)
    if not m:
        return insert_sql

    table = m.group(1)
    rest = insert_sql[m.end():]
    cp = rest.find('(')
    if cp == -1:
        return insert_sql
    cols_str, after_cols = parse_parens(rest, cp)

    vp = after_cols.upper().find('VALUES')
    if vp == -1:
        return insert_sql
    before_vals = after_cols[:vp]
    vals_start_text = after_cols[vp:]
    vals_open = vals_start_text.find('(')
    if vals_open == -1:
        return insert_sql
    vals_str, after_vals = parse_parens(vals_start_text, vals_open)

    cols = split_commas_ignoring_parens(cols_str)
    vals = split_commas_ignoring_parens(vals_str)

    new_cols = []
    new_vals = []
    for c, v in zip(cols, vals):
        if c.strip().lower().endswith('tenant_id') or c.strip().lower() == 'tenant_id':
            continue
        new_cols.append(c.strip())
        new_vals.append(v.strip())

    return (
        f'INSERT INTO {table} ({', '.join(new_cols)}) VALUES ({', '.join(new_vals)})' +
        before_vals + after_vals
    )


def remove_tenant_from_insert(sql):
    """Remove tenant_id from INSERT column list and the matching VALUES entry."""
    pattern = re.compile(r'INSERT\s+INTO\s+(\w+)\s*', re.IGNORECASE | re.DOTALL)
    m = pattern.search(sql)
    if not m:
        return sql
    transformed = _transform_one_insert(sql[m.start():])
    return sql[:m.start()] + transformed


def split_commas_ignoring_parens(s):
    """Split a comma-separated list, ignoring commas inside parentheses."""
    parts = []
    depth = 0
    start = 0
    for i, ch in enumerate(s):
        if ch == '(':
            depth += 1
        elif ch == ')':
            depth -= 1
        elif ch == ',' and depth == 0:
            parts.append(s[start:i])
            start = i + 1
    parts.append(s[start:])
    return parts


def renumber_positional(sql):
    """Renumber remaining positional parameters after tenant params removed."""
    nums = [int(m) for m in re.findall(r'\$(\d+)', sql)]
    if not nums:
        return sql
    old_max = max(nums)
    present = set(nums)
    missing = {n for n in range(1, old_max + 1) if n not in present}

    def replace(m):
        n = int(m.group(1))
        new = n - sum(1 for x in missing if x < n)
        return f'${new}'

    return re.sub(r'\$(\d+)', replace, sql)


def transform_query(sql):
    sql = sql.strip()
    sql = remove_tenant_from_on_conflict(sql)
    sql = remove_tenant_from_insert(sql)
    sql = remove_tenant_from_lists(sql)
    sql = remove_tenant_conditions(sql)
    sql = renumber_positional(sql)
    sql = clean_whitespace(sql)
    return sql


def main(path):
    with open(path, 'r') as f:
        text = f.read()

    queries, leading = split_queries(text)
    out = [leading] if leading else []
    for header, leading_comments, sql, trailing in queries:
        transformed = transform_query(sql)
        out.append(header + '\n' + leading_comments + transformed + trailing)

    output = ''.join(out)
    # Collapse more than one consecutive blank line.
    output = re.sub(r'\n\s*\n\s*\n', '\n\n', output)
    print(output, end='')


if __name__ == '__main__':
    main(sys.argv[1])
