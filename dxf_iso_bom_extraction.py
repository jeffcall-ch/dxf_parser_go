import os
import csv
import ezdxf
import re

# Global debug flag - set to False to suppress debug output
DEBUG_MODE = False

# Compile regex patterns once for better performance
PIECE_NUMBER_PATTERN = re.compile(r'^<\d+>$')
NUMBER_PATTERN = re.compile(r'^\d+(\.\d+)?$')
KKS_PATTERN = re.compile(r'\b\d[A-Z]{3}\d{2}BR\d{3}\b')

def debug_print(message):
    """Print debug message only if DEBUG_MODE is True"""
    if DEBUG_MODE:
        print(message)

def is_piece_number(text):
    """Check if text is a piece number like <1>, <2>, etc."""
    return bool(PIECE_NUMBER_PATTERN.match(str(text).strip()))

def is_number(text):
    """Check if text is a number (can include decimal)"""
    try:
        float(str(text).strip())
        return True
    except ValueError:
        return False

def is_text_remark(text):
    """Check if text is a remark (not a piece number or pure number)"""
    text_str = str(text).strip()
    if not text_str:
        return False
    return not is_piece_number(text_str) and not is_number(text_str)

def validate_and_correct_cut_length_row(row):
    """
    Validate and correct a CUT PIPE LENGTH row to ensure proper column alignment.
    Expected structure: [PIECE NO, CUT LENGTH, N.S.(MM), REMARKS, PIECE NO, CUT LENGTH, N.S.(MM), REMARKS]
    Each piece should have exactly 3 values: PIECE NO, CUT LENGTH, N.S.(MM)
    """
    if not row:
        return row
    
    # Create an 8-column template
    corrected = [''] * 8
    
    # Find piece numbers first to establish groupings
    pieces = []
    for i, cell in enumerate(row):
        cell_str = str(cell).strip()
        if is_piece_number(cell_str):
            pieces.append((i, cell_str))
    
    debug_print(f"[DEBUG] Row validation - Found pieces: {pieces}")
    
    if len(pieces) == 0:
        # No pieces found, return original row
        return row[:8] + [''] * (8 - len(row)) if len(row) < 8 else row[:8]
    
    # Group data by piece based on original positions
    piece_groups = []
    
    for piece_idx, (piece_pos, piece_name) in enumerate(pieces):
        group = {'piece': piece_name, 'numbers': [], 'remarks': []}
        
        # Determine the range for this piece
        if piece_idx + 1 < len(pieces):
            next_piece_pos = pieces[piece_idx + 1][0]
            end_pos = next_piece_pos
        else:
            end_pos = len(row)
        
        # Collect numbers and remarks between this piece and the next
        for i in range(piece_pos + 1, end_pos):
            if i < len(row):
                cell_str = str(row[i]).strip()
                if not cell_str:
                    continue
                    
                if is_number(cell_str):
                    group['numbers'].append(cell_str)
                elif is_text_remark(cell_str):
                    group['remarks'].append(cell_str)
        
        piece_groups.append(group)
    
    debug_print(f"[DEBUG] Piece groups: {piece_groups}")
    
    # Fill the corrected row
    for group_idx, group in enumerate(piece_groups[:2]):  # Max 2 pieces per row
        base_col = group_idx * 4  # 0 for first piece, 4 for second piece
        
        # Place piece number
        corrected[base_col] = group['piece']
        
        # Place numbers (should be CUT LENGTH and N.S.)
        # Each piece should have exactly 2 numbers: CUT LENGTH and N.S.(MM)
        for num_idx, number in enumerate(group['numbers'][:2]):
            corrected[base_col + 1 + num_idx] = number
        
        # Place remarks
        if group['remarks']:
            corrected[base_col + 3] = group['remarks'][0]  # First remark only
    
    debug_print(f"[DEBUG] Corrected row: {corrected}")
    return corrected

def extract_text_entities(doc):
    # Returns list of (text, x, y) for all text entities
    entities = []
    for e in doc.modelspace():
        if e.dxftype() == 'MTEXT':
            text = e.plain_text()
            x, y = e.dxf.insert.x, e.dxf.insert.y
            entities.append((text.strip(), x, y))
        elif e.dxftype() == 'TEXT':
            text = e.dxf.text
            x, y = e.dxf.insert.x, e.dxf.insert.y
            entities.append((text.strip(), x, y))
    
    # Sort entities once by Y coordinate (descending) for faster processing
    entities.sort(key=lambda item: -item[2])
    return entities

def find_pipe_class(text_entities):
    """
    Find pipe class from DESIGN DATA section at bottom center of drawing.
    Pipe class pattern is exactly 4 uppercase letters (AAAA) like 'AHDX'.
    """
    # Compile pattern for exactly 4 uppercase letters
    pipe_class_pattern = re.compile(r'\b[A-Z]{4}\b')
    
    # Look for 'Pipe class:' label first
    pipe_class_label_y = None
    pipe_class_label_x = None
    
    for text, x, y in text_entities:
        text_clean = text.strip().replace(' ', '').replace('\n', '').lower()
        # More flexible matching for pipe class label
        if ('pipeclass' in text_clean or 'pipe_class' in text_clean or 
            ('pipe' in text.lower() and 'class' in text.lower())):
            pipe_class_label_x, pipe_class_label_y = x, y
            debug_print(f"[DEBUG] Found pipe class label at X={x}, Y={y}: '{text}'")
            break
    
    if pipe_class_label_y is not None:
        # Look for 4-letter codes near the label
        candidates = []
        for text, x, y in text_entities:
            # Look for text near the label (horizontally close, similar Y level)
            if (abs(y - pipe_class_label_y) < 20 and  # Same row or close
                x > pipe_class_label_x and  # To the right of label
                abs(x - pipe_class_label_x) < 200):  # Not too far horizontally
                text_clean = text.strip()
                match = pipe_class_pattern.search(text_clean)
                if match:
                    candidates.append((match.group(), abs(x - pipe_class_label_x)))
                    debug_print(f"[DEBUG] Pipe class candidate: '{match.group()}' at distance {abs(x - pipe_class_label_x)}")
        
        if candidates:
            # Sort by distance from label and pick the closest one
            candidates.sort(key=lambda item: item[1])
            pipe_class = candidates[0][0]
            debug_print(f"[DEBUG] Selected pipe class: '{pipe_class}'")
            return pipe_class
    
    # Alternative approach: Look for DESIGN DATA section first, then find pipe class within it
    design_data_y = None
    for text, x, y in text_entities:
        if 'DESIGN DATA' in text.upper():
            design_data_y = y
            debug_print(f"[DEBUG] Found DESIGN DATA at Y={y}")
            break
    
    if design_data_y is not None:
        # Look for 4-letter codes within DESIGN DATA area (below the title)
        design_area_entities = [(text, x, y) for text, x, y in text_entities 
                              if y < design_data_y and y > design_data_y - 150]  # Within 150 units below DESIGN DATA
        
        for text, x, y in design_area_entities:
            text_clean = text.strip()
            match = pipe_class_pattern.search(text_clean)
            if match:
                pipe_class = match.group()
                debug_print(f"[DEBUG] Found pipe class in DESIGN DATA area: '{pipe_class}' at X={x}, Y={y}")
                return pipe_class
    
    # Fallback: look for 4-letter codes in bottom center area
    # Focus search on bottom half and center area of drawing
    bottom_entities = [e for e in text_entities if e[2] < 100]  # Y < 100 (bottom area)
    if not bottom_entities:
        bottom_entities = sorted(text_entities, key=lambda e: e[2])[:len(text_entities)//2]
    
    # Look for candidates in center area (avoid far right where revision notes might be)
    center_candidates = []
    for text, x, y in bottom_entities:
        if x < 500:  # Avoid far right area where revision notes typically are
            match = pipe_class_pattern.search(text.strip())
            if match:
                potential_class = match.group()
                center_candidates.append((potential_class, x, y))
                debug_print(f"[DEBUG] Center area pipe class candidate: '{potential_class}' at X={x}, Y={y}")
    
    if center_candidates:
        # Prefer candidates in the center-left area (where DESIGN DATA typically is)
        center_candidates.sort(key=lambda item: item[1])  # Sort by X coordinate
        pipe_class = center_candidates[0][0]
        debug_print(f"[DEBUG] Selected center area pipe class: '{pipe_class}'")
        return pipe_class
    
    debug_print(f"[DEBUG] No pipe class found")
    return ''

def find_drawing_no(text_entities):
    # Find KKS code with pattern 1AAA11BR111 (1=digit, A=capital letter, BR=fixed)
    # Located in bottom right corner, below and to the right of ERECTION MATERIALS
    import re
    
    # First find ERECTION MATERIALS position to establish search area
    erection_x, erection_y = None, None
    for text, x, y in text_entities:
        if 'ERECTION MATERIALS' in text.upper():
            erection_x, erection_y = x, y
            debug_print(f"[DEBUG] Found ERECTION MATERIALS at X={x}, Y={y}")
            break
    
    # Use pre-compiled KKS pattern for better performance
    candidates = []
    for text, x, y in text_entities:
        match = KKS_PATTERN.search(text)
        if match:
            kks_code = match.group()
            # If we found ERECTION MATERIALS, filter by position (below and to the right)
            if erection_x is not None and erection_y is not None:
                if x >= erection_x and y <= erection_y:  # Right and below
                    candidates.append((kks_code, x, y, abs(y)))  # Use abs(y) for sorting (lower Y = further down)
                    debug_print(f"[DEBUG] Found KKS candidate '{kks_code}' at X={x}, Y={y}")
            else:
                # If no ERECTION MATERIALS found, consider all KKS codes
                candidates.append((kks_code, x, y, abs(y)))
                debug_print(f"[DEBUG] Found KKS candidate '{kks_code}' at X={x}, Y={y} (no ERECTION MATERIALS reference)")
    
    if candidates:
        # Sort by Y coordinate (lowest Y = bottom of drawing) and pick the first one
        candidates.sort(key=lambda item: item[3])  # Sort by abs(y)
        selected_kks = candidates[0][0]
        debug_print(f"[DEBUG] Selected KKS code: '{selected_kks}'")
        return selected_kks
    
    # Fallback: try to find Drawing-No. field if no KKS found
    for i, (text, x, y) in enumerate(text_entities):
        if 'Drawing-No.' in text:
            # Look for next text entity to the right or below
            for t, tx, ty in text_entities[i+1:i+5]:
                if tx > x or ty < y:
                    debug_print(f"[DEBUG] Fallback: Found Drawing-No. '{t}'")
                    return t
    
    debug_print(f"[DEBUG] No KKS code or Drawing-No. found")
    return ''

def convert_cut_length_to_single_row_format(header, rows, drawing_no, pipe_class):
    """
    Convert CUT PIPE LENGTH from 8-column format (2 pieces per row) 
    to 5-column format (1 piece per row)
    """
    if not rows:
        return ['PIECE NO', 'CUT LENGTH', 'N.S. (MM)', 'REMARKS', 'Drawing-No.', 'Pipe Class'], []
    
    # New header format with pipe class
    new_header = ['PIECE NO', 'CUT LENGTH', 'N.S. (MM)', 'REMARKS', 'Drawing-No.', 'Pipe Class']
    new_rows = []
    
    for row in rows:
        # Extract first piece (columns 0-3)
        if len(row) >= 4:
            piece1_no = row[0] if row[0].strip() else ''
            piece1_length = row[1] if len(row) > 1 and row[1].strip() else ''
            piece1_ns = row[2] if len(row) > 2 and row[2].strip() else ''
            piece1_remarks = row[3] if len(row) > 3 and row[3].strip() else ''
            
            if piece1_no:  # Only add if piece number exists
                new_rows.append([piece1_no, piece1_length, piece1_ns, piece1_remarks, drawing_no, pipe_class])
        
        # Extract second piece (columns 4-7)
        if len(row) >= 8:
            piece2_no = row[4] if row[4].strip() else ''
            piece2_length = row[5] if len(row) > 5 and row[5].strip() else ''
            piece2_ns = row[6] if len(row) > 6 and row[6].strip() else ''
            piece2_remarks = row[7] if len(row) > 7 and row[7].strip() else ''
            
            if piece2_no:  # Only add if piece number exists
                new_rows.append([piece2_no, piece2_length, piece2_ns, piece2_remarks, drawing_no, pipe_class])
    
    return new_header, new_rows

def process_dxf_file(filepath):
    try:
        debug_print(f"[DEBUG] Opening DXF file: {filepath}")
        doc = ezdxf.readfile(filepath)
        text_entities = extract_text_entities(doc)
        drawing_no = find_drawing_no(text_entities)
        pipe_class = find_pipe_class(text_entities)
        mat_header, mat_rows = extract_table(text_entities, 'ERECTION MATERIALS')
        cut_header, cut_rows = extract_table(text_entities, 'CUT PIPE LENGTH')
        # Add Drawing-No. and Pipe Class to each row
        if mat_rows:
            mat_header_out = mat_header + ['Drawing-No.', 'Pipe Class']
            mat_rows_out = [r + [drawing_no, pipe_class] for r in mat_rows]
        else:
            mat_header_out, mat_rows_out = [], []
        if cut_rows:
            # Convert to single-row format (one piece per row) with pipe class
            cut_header_converted, cut_rows_converted = convert_cut_length_to_single_row_format(cut_header, cut_rows, drawing_no, pipe_class)
            cut_header_out = cut_header_converted
            cut_rows_out = cut_rows_converted
        else:
            cut_header_out, cut_rows_out = [], []
        debug_print(f"[DEBUG] Extracted {len(mat_rows_out)} material rows and {len(cut_rows_out)} cut length rows from {filepath}")
        debug_print(f"[DEBUG] Drawing No: '{drawing_no}', Pipe Class: '{pipe_class}'")
        return {
            'drawing_no': drawing_no,
            'pipe_class': pipe_class,
            'mat_header': mat_header_out,
            'mat_rows': mat_rows_out,
            'cut_header': cut_header_out,
            'cut_rows': cut_rows_out,
            'error': None
        }
    except Exception as e:
        print(f"[ERROR] Failed to process {filepath}: {e}")
        return {
            'drawing_no': '',
            'pipe_class': '',
            'mat_header': [],
            'mat_rows': [],
            'cut_header': [],
            'cut_rows': [],
            'error': str(e)
        }

def extract_table(text_entities, table_title, max_cols=20, max_rows=100):
    # Find table by title, then extract rows/columns by y/x positions
    # Returns: [header], [rows]
    # Find title
    title_entity = None
    start_x = None
    title_y = None
    for text, x, y in text_entities:
        if table_title.lower() in text.lower():
            start_x = x  # Use the start of the DXF entity
            title_y = y
            debug_print(f"[DEBUG] Table title '{table_title}' found at X={x}, Y={y}, text='{text}'")
            title_entity = (text, x, y)
            break
    
    if not title_entity or start_x is None:
        debug_print(f"[DEBUG] Table title '{table_title}' not found.")
        return [], []
    
    title_text, title_x, title_y = title_entity
    # Optimize filtering with early breaks and efficient conditions
    filtered_entities = []
    if table_title.lower() == 'cut pipe length':
        # Allow data to the left of the title for cut pipe length table
        min_x = title_x - 50
        for text, x, y in text_entities:
            if y >= title_y:  # Skip rows at or above title
                continue
            if x >= min_x:
                filtered_entities.append((text, x, y))
    else:
        # Only keep entities with x >= title_x and y < title_y
        for text, x, y in text_entities:
            if y >= title_y:  # Skip rows at or above title
                continue
            if x >= title_x:
                filtered_entities.append((text, x, y))
    
    # Now process table from filtered_entities only
    rows_dict = {}
    for text, x, y in filtered_entities:
        y_key = round(y, 1)
        if y_key not in rows_dict:
            rows_dict[y_key] = []
        rows_dict[y_key].append((x, text))
    
    sorted_rows = sorted(rows_dict.items(), key=lambda item: -item[0])
    
    # For each row, sort by x ascending (left to right)
    table_rows = []
    for idx, (y, cells) in enumerate(sorted_rows):
        row = [t for x, t in sorted(cells, key=lambda c: c[0])]
        
        # Reduced debug output for performance (only show first few rows and special cases)
        if table_title.lower() == 'cut pipe length' and idx == 2:
            xs = [x for x, t in sorted(cells, key=lambda c: c[0])]
            debug_print(f"[DEBUG] Extracted row {idx+1} at y={y}, x={xs}: {row} <-- 3RD ROW BELOW 'CUT PIPE LENGTH'")
        elif idx < 3:  # Only show first 3 rows for debugging
            xs = [x for x, t in sorted(cells, key=lambda c: c[0])]
            debug_print(f"[DEBUG] Extracted row {idx+1} at y={y}, x={xs}: {row}")
        
        table_rows.append(row)
    
    if table_title.lower() == 'cut pipe length':
        debug_print(f"[DEBUG] Total rows extracted for 'CUT PIPE LENGTH': {len(table_rows)}")
    # (Removed duplicate/old code using table_entities)
    # Heuristic: first two rows are headers, merge them
    if len(table_rows) >= 2:
        header = []
        for i in range(max(len(table_rows[0]), len(table_rows[1]))):
            h1 = table_rows[0][i] if i < len(table_rows[0]) else ''
            h2 = table_rows[1][i] if i < len(table_rows[1]) else ''
            # Special handling for CUT PIPE LENGTH table headers
            if table_title.lower() == 'cut pipe length':
                # For CUT PIPE LENGTH, handle the specific pattern more carefully
                if h1 and h2:
                    if h1 == 'N.S.' and h2 == '(MM)':
                        merged = 'N.S. (MM)'
                    elif h1 == 'PIECE' and h2 == 'NO':
                        merged = 'PIECE NO'
                    elif h1 == 'CUT' and h2 == 'LENGTH':
                        merged = 'CUT LENGTH'
                    elif h1 == 'REMARKS' and h2 == 'NO':
                        merged = 'REMARKS'  # Don't add 'NO' to REMARKS
                    elif h1 == 'REMARKS' and not h2:
                        merged = 'REMARKS'
                    # Handle the second set of columns (right side)
                    elif h1 == 'PIECE' and h2 == 'LENGTH':  # Should be 'PIECE NO'
                        merged = 'PIECE NO'
                    elif h1 == 'CUT' and h2 == '(MM)':  # Should be 'CUT LENGTH'
                        merged = 'CUT LENGTH'
                    else:
                        merged = f"{h1} {h2}".strip()
                elif h1 and not h2:
                    # Handle single header values for the right side columns
                    if h1 == 'PIECE':
                        merged = 'PIECE NO'
                    elif h1 == 'CUT':
                        merged = 'CUT LENGTH'
                    elif h1 == 'N.S.':
                        merged = 'N.S. (MM)'
                    else:
                        merged = h1
                elif h2 and not h1:
                    merged = h2
                else:
                    merged = ''
            else:
                merged = f"{h1} {h2}".strip() if h1 or h2 else ''
            header.append(merged)
        data_rows = table_rows[2:]
    else:
        header = table_rows[0] if table_rows else []
        data_rows = table_rows[1:] if len(table_rows) > 1 else []
    # For ERECTION MATERIALS, stop at 'TOTAL WEIGHT' row
    if 'ERECTION MATERIALS' in table_title.upper():
        end_idx = None
        for i, row in enumerate(data_rows):
            if any('TOTAL WEIGHT' in str(cell).upper() for cell in row):
                end_idx = i + 1
                break
        if end_idx:
            data_rows = data_rows[:end_idx]
        
        # Process ERECTION MATERIALS to move categories from column A to column F
        processed_rows = []
        current_category = ""
        
        for row in data_rows:
            # Check if this row is a category header (has content in first column but empty in other key columns)
            if row and row[0] and (len(row) < 2 or not row[1] or len(row) < 4 or not row[3]):
                # This is likely a category header like "PIPE", "FITTINGS", etc.
                if row[0] not in ["TOTAL ERECTION WEIGHT", "TOTAL WEIGHT"]:
                    current_category = row[0]
                    debug_print(f"[DEBUG] Found category: '{current_category}'")
                    continue  # Skip category header rows, don't add to processed_rows
                else:
                    # For total rows, move the total type to column F and weight value to column E
                    new_row = row[:]
                    total_type = new_row[0]  # Save the total type
                    weight_value = new_row[1] if len(new_row) > 1 else ""  # Save the weight value from column B
                    new_row[0] = ""  # Clear column A
                    if len(new_row) > 1:
                        new_row[1] = ""  # Clear column B
                    while len(new_row) < 6:
                        new_row.append("")
                    new_row[4] = weight_value  # Put weight value in column E (WEIGHT column)
                    new_row.insert(5, total_type)  # Insert total type at position 5 (column F)
                    processed_rows.append(new_row)
            else:
                # This is a regular data row, add the current category to column F
                new_row = row[:]
                while len(new_row) < 6:
                    new_row.append("")
                new_row.insert(5, current_category)  # Insert category at position 5 (column F)
                processed_rows.append(new_row)
        
        data_rows = processed_rows
        
        # Update header to include the new CATEGORY column
        if header:
            header.insert(5, "CATEGORY")  # Insert at position 5 (column F)
    # For CUT PIPE LENGTH, filter rows with '<' 
    if table_title.lower() == 'cut pipe length':
        kept_rows = []
        for row in data_rows:
            if '<' in ''.join(row):
                kept_rows.append(row)
        debug_print(f"[DEBUG] Kept rows for 'CUT PIPE LENGTH':")
        for r in kept_rows[:2]:  # Only show first 2 rows for performance
            debug_print(f"[DEBUG] {r}")
        data_rows = kept_rows
        
        # Apply column validation and correction for CUT PIPE LENGTH
        corrected_rows = []
        for row in data_rows:
            corrected_row = validate_and_correct_cut_length_row(row)
            corrected_rows.append(corrected_row)
        data_rows = corrected_rows
    # For CUT PIPE LENGTH, stop at first empty row
    if 'CUT PIPE LENGTH' in table_title.upper():
        new_data_rows = []
        for row in data_rows:
            if all(cell.strip() == '' for cell in row):
                break
            new_data_rows.append(row)
        data_rows = new_data_rows
    # Pad all rows to header length, keep empty cells
    padded_rows = []
    for row in data_rows:
        padded = row + [''] * (len(header) - len(row)) if len(row) < len(header) else row[:len(header)]
        padded_rows.append(padded)
    return header, padded_rows

def main(directory, debug=False, workers=None):
    global DEBUG_MODE
    DEBUG_MODE = debug
    
    import time
    start_time = time.time()
    
    material_rows = []
    cut_rows = []
    mat_header = None
    cut_header = None
    summary = []
    
    # Count DXF files first
    dxf_files = []
    for root, _, files in os.walk(directory):
        for file in files:
            if file.lower().endswith('.dxf'):
                dxf_files.append(os.path.join(root, file))
    
    total_files = len(dxf_files)
    debug_print(f"[DEBUG] Found {total_files} DXF files to process")
    
    if total_files == 0:
        print("No DXF files found.")
        return
    
    # Determine if we should use parallel processing
    if workers is None:
        # Auto-determine: use parallel processing for multiple files
        if total_files > 1:
            import multiprocessing as mp
            workers = min(total_files, mp.cpu_count())
        else:
            workers = 1
    
    if workers > 1:
        print(f"Processing {total_files} DXF files using {workers} parallel workers...")
        results = process_files_parallel(dxf_files, workers, debug)
    else:
        print(f"Processing {total_files} DXF files sequentially...")
        results = process_files_sequential(dxf_files, debug)
    
    # Aggregate results
    successful_files = 0
    total_processing_time = 0
    
    for result in results:
        if result['mat_rows']:
            if not mat_header:
                mat_header = result['mat_header']
            material_rows.extend(result['mat_rows'])
        
        if result['cut_rows']:
            if not cut_header:
                cut_header = result['cut_header']
            cut_rows.extend(result['cut_rows'])
        
        summary_row = {
            'file_path': result.get('file_path', ''),
            'filename': result.get('filename', ''),
            'drawing_no': result['drawing_no'],
            'pipe_class': result['pipe_class'],
            'mat_rows': len(result['mat_rows']),
            'cut_rows': len(result['cut_rows']),
            'mat_missing': not bool(result['mat_rows']),
            'cut_missing': not bool(result['cut_rows']),
            'error': result['error'],
            'processing_time': result.get('processing_time', 0)
        }
        summary.append(summary_row)
        
        if not result['error']:
            successful_files += 1
            total_processing_time += result.get('processing_time', 0)
    
    # Write CSVs
    write_output_files(directory, material_rows, cut_rows, summary, mat_header, cut_header)
    
    # Final timing summary
    end_time = time.time()
    total_time = end_time - start_time
    print_final_summary(total_files, successful_files, total_time, total_processing_time, 
                       workers, len(material_rows), len(cut_rows), directory)

def process_files_sequential(dxf_files, debug):
    """Process files one by one sequentially."""
    import time
    results = []
    for i, path in enumerate(dxf_files, 1):
        file = os.path.basename(path)
        file_start_time = time.time()
        debug_print(f"[DEBUG] Processing file {i}/{len(dxf_files)}: {file}")
        print(f"Processing file {i}/{len(dxf_files)}: {file}")
        
        result = process_dxf_file(path)
        
        file_end_time = time.time()
        file_time = file_end_time - file_start_time
        result['processing_time'] = file_time
        result['filename'] = file
        result['file_path'] = path
        
        debug_print(f"[DEBUG] File {i}/{len(dxf_files)} completed in {file_time:.2f}s")
        results.append(result)
    
    return results

def process_files_parallel(dxf_files, workers, debug):
    """Process files using multiprocessing."""
    from concurrent.futures import ProcessPoolExecutor, as_completed
    import time
    
    results = []
    completed_count = 0
    total_files = len(dxf_files)
    
    with ProcessPoolExecutor(max_workers=workers) as executor:
        # Submit all jobs
        future_to_file = {executor.submit(process_dxf_file_with_timing, file_path): file_path 
                         for file_path in dxf_files}
        
        # Process completed jobs as they finish
        for future in as_completed(future_to_file):
            file_path = future_to_file[future]
            completed_count += 1
            
            try:
                result = future.result()
                results.append(result)
                
                filename = os.path.basename(file_path)
                processing_time = result.get('processing_time', 0)
                print(f"[{completed_count}/{total_files}] Completed: {filename} ({processing_time:.2f}s)")
                
            except Exception as e:
                print(f"[{completed_count}/{total_files}] Failed: {os.path.basename(file_path)} - {e}")
                # Add error result
                results.append({
                    'filename': os.path.basename(file_path),
                    'file_path': file_path,
                    'drawing_no': '',
                    'pipe_class': '',
                    'mat_header': [],
                    'mat_rows': [],
                    'cut_header': [],
                    'cut_rows': [],
                    'error': str(e),
                    'processing_time': 0
                })
    
    return results

def process_dxf_file_with_timing(filepath):
    """Process a single DXF file and return results with timing information."""
    import time
    start_time = time.time()
    
    result = process_dxf_file(filepath)
    processing_time = time.time() - start_time
    
    # Add timing and file info
    result['processing_time'] = processing_time
    result['filename'] = os.path.basename(filepath)
    result['file_path'] = filepath
    
    return result

def write_output_files(directory, material_rows, cut_rows, summary, mat_header, cut_header):
    """Write all output CSV files."""
    # Write all_materials.csv
    out_path = os.path.join(directory, 'all_materials.csv')
    with open(out_path, 'w', newline='', encoding='utf-8') as f:
        writer = csv.writer(f)
        if material_rows:
            writer.writerow(mat_header)
            writer.writerows(material_rows)
        else:
            writer.writerow(['No Data'])
    
    # Write all_cut_lengths.csv
    out_path = os.path.join(directory, 'all_cut_lengths.csv')
    with open(out_path, 'w', newline='', encoding='utf-8') as f:
        writer = csv.writer(f)
        if cut_rows:
            writer.writerow(cut_header)
            writer.writerows(cut_rows)
        else:
            writer.writerow(['No Data'])
    
    # Write summary.csv
    out_path = os.path.join(directory, 'summary.csv')
    with open(out_path, 'w', newline='', encoding='utf-8') as f:
        writer = csv.DictWriter(f, fieldnames=['file_path', 'filename', 'drawing_no', 'pipe_class', 'mat_rows', 'cut_rows', 'mat_missing', 'cut_missing', 'error', 'processing_time'])
        writer.writeheader()
        writer.writerows(summary)

def print_final_summary(total_files, successful_files, total_time, total_processing_time, 
                       workers, total_materials, total_cuts, directory):
    """Print comprehensive final summary."""
    print(f"\n=== PROCESSING COMPLETE ===")
    print(f"Total files processed: {total_files}")
    print(f"Successful: {successful_files}")
    print(f"Failed: {total_files - successful_files}")
    print(f"Workers used: {workers}")
    print(f"Total wall time: {total_time:.2f} seconds")
    
    if workers > 1 and successful_files > 0:
        avg_per_file = total_processing_time / successful_files if successful_files > 0 else 0
        speedup = total_processing_time / total_time if total_time > 0 else 1
        efficiency = speedup / workers * 100 if workers > 0 else 0
        
        print(f"Total processing time: {total_processing_time:.2f} seconds")
        print(f"Average per file: {avg_per_file:.2f} seconds")
        print(f"Speedup factor: {speedup:.2f}x")
        print(f"Parallel efficiency: {efficiency:.1f}%")
        time_saved = total_processing_time - total_time
        print(f"Time saved: {time_saved:.2f} seconds")
    elif successful_files > 0:
        avg_per_file = total_time / successful_files
        print(f"Average time per file: {avg_per_file:.2f} seconds")
    
    print(f"Total material rows: {total_materials}")
    print(f"Total cut length rows: {total_cuts}")
    print(f"Output files written to: {directory}")
    
    debug_print(f"[DEBUG] === PROCESSING COMPLETE ===")
    debug_print(f"[DEBUG] Total files processed: {total_files}")
    debug_print(f"[DEBUG] Total wall time: {total_time:.2f} seconds")
    debug_print(f"[DEBUG] Total material rows: {total_materials}")
    debug_print(f"[DEBUG] Total cut length rows: {total_cuts}")
    debug_print(f"[DEBUG] Output files written to: {directory}")

if __name__ == '__main__':
    import sys
    import argparse
    
    parser = argparse.ArgumentParser(description='Extract DXF Isometric BOM data')
    parser.add_argument('directory', help='Directory containing DXF files (recursively searched)')
    parser.add_argument('--debug', action='store_true', help='Enable detailed debug output')
    parser.add_argument('--workers', type=int, help='Number of parallel workers (default: auto-detect based on file count)')
    
    args = parser.parse_args()
    
    if not os.path.isdir(args.directory):
        print(f"Error: Directory '{args.directory}' does not exist")
        sys.exit(1)
    
    main(args.directory, debug=args.debug, workers=args.workers)
