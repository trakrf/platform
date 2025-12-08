SET search_path=trakrf,public;

UPDATE locations
SET identifier = CASE identifier
    WHEN 'WAREHOUSE_1' THEN 'WAREHOUSE-1'
    WHEN 'OFFICE_1' THEN 'OFFICE-1'
    WHEN 'LAB_1' THEN 'LAB-1'
    ELSE identifier
END
WHERE identifier IN ('WAREHOUSE_1', 'OFFICE_1', 'LAB_1');
