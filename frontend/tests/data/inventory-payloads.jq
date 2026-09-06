[.[] 
 | select(.type == "notification" and (.data | test("81 00"))) 
 | .data
 | split(" ")
 | .[10:] 
 | map("0x" + . + ",")]
