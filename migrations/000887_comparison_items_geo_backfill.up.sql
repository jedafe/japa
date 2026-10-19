-- Backfill comparison_items.geo_space_id for intel cost-of-living rows.
-- comparison_items has no `country` text column, so resolution uses three
-- paths in order: the linked contents row's geo_space_id, the item title
-- matched against a country space (geo_code ISO-2, ISO-3 map, name), and
-- the title matched against a city/state space resolved up its parent
-- chain to the country ancestor. Rows with no geo signal (institution and
-- service comparisons) correctly stay NULL.

-- Pass 1: inherit from the linked contents row.
UPDATE comparison_items c
SET geo_space_id = co.geo_space_id
FROM contents co
WHERE c.geo_space_id IS NULL
  AND co.id = c.content_id
  AND co.geo_space_id IS NOT NULL;

-- Pass 2: title -> country space (ISO-2, ISO-3->ISO-2, or exact name).
WITH iso3(iso3_code, iso2_code) AS (VALUES ('AFG','AF'), ('ALA','AX'), ('ALB','AL'), ('DZA','DZ'), ('ASM','AS'), ('AND','AD'), ('AGO','AO'), ('AIA','AI'), ('ATA','AQ'), ('ATG','AG'), ('ARG','AR'), ('ARM','AM'), ('ABW','AW'), ('AUS','AU'), ('AUT','AT'), ('AZE','AZ'), ('BHS','BS'), ('BHR','BH'), ('BGD','BD'), ('BRB','BB'), ('BLR','BY'), ('BEL','BE'), ('BLZ','BZ'), ('BEN','BJ'), ('BMU','BM'), ('BTN','BT'), ('BOL','BO'), ('BES','BQ'), ('BIH','BA'), ('BWA','BW'), ('BVT','BV'), ('BRA','BR'), ('IOT','IO'), ('BRN','BN'), ('BGR','BG'), ('BFA','BF'), ('BDI','BI'), ('CPV','CV'), ('KHM','KH'), ('CMR','CM'), ('CAN','CA'), ('CYM','KY'), ('CAF','CF'), ('TCD','TD'), ('CHL','CL'), ('CHN','CN'), ('CXR','CX'), ('CCK','CC'), ('COL','CO'), ('COM','KM'), ('COG','CG'), ('COD','CD'), ('COK','CK'), ('CRI','CR'), ('CIV','CI'), ('HRV','HR'), ('CUB','CU'), ('CUW','CW'), ('CYP','CY'), ('CZE','CZ'), ('DNK','DK'), ('DJI','DJ'), ('DMA','DM'), ('DOM','DO'), ('ECU','EC'), ('EGY','EG'), ('SLV','SV'), ('GNQ','GQ'), ('ERI','ER'), ('EST','EE'), ('SWZ','SZ'), ('ETH','ET'), ('FLK','FK'), ('FRO','FO'), ('FJI','FJ'), ('FIN','FI'), ('FRA','FR'), ('GUF','GF'), ('PYF','PF'), ('ATF','TF'), ('GAB','GA'), ('GMB','GM'), ('GEO','GE'), ('DEU','DE'), ('GHA','GH'), ('GIB','GI'), ('GRC','GR'), ('GRL','GL'), ('GRD','GD'), ('GLP','GP'), ('GUM','GU'), ('GTM','GT'), ('GGY','GG'), ('GIN','GN'), ('GNB','GW'), ('GUY','GY'), ('HTI','HT'), ('HMD','HM'), ('VAT','VA'), ('HND','HN'), ('HKG','HK'), ('HUN','HU'), ('ISL','IS'), ('IND','IN'), ('IDN','ID'), ('IRN','IR'), ('IRQ','IQ'), ('IRL','IE'), ('IMN','IM'), ('ISR','IL'), ('ITA','IT'), ('JAM','JM'), ('JPN','JP'), ('JEY','JE'), ('JOR','JO'), ('KAZ','KZ'), ('KEN','KE'), ('KIR','KI'), ('PRK','KP'), ('KOR','KR'), ('KWT','KW'), ('KGZ','KG'), ('LAO','LA'), ('LVA','LV'), ('LBN','LB'), ('LSO','LS'), ('LBR','LR'), ('LBY','LY'), ('LIE','LI'), ('LTU','LT'), ('LUX','LU'), ('MAC','MO'), ('MDG','MG'), ('MWI','MW'), ('MYS','MY'), ('MDV','MV'), ('MLI','ML'), ('MLT','MT'), ('MHL','MH'), ('MTQ','MQ'), ('MRT','MR'), ('MUS','MU'), ('MYT','YT'), ('MEX','MX'), ('FSM','FM'), ('MDA','MD'), ('MCO','MC'), ('MNG','MN'), ('MNE','ME'), ('MSR','MS'), ('MAR','MA'), ('MOZ','MZ'), ('MMR','MM'), ('NAM','NA'), ('NRU','NR'), ('NPL','NP'), ('NLD','NL'), ('NCL','NC'), ('NZL','NZ'), ('NIC','NI'), ('NER','NE'), ('NGA','NG'), ('NIU','NU'), ('NFK','NF'), ('MKD','MK'), ('MNP','MP'), ('NOR','NO'), ('OMN','OM'), ('PAK','PK'), ('PLW','PW'), ('PSE','PS'), ('PAN','PA'), ('PNG','PG'), ('PRY','PY'), ('PER','PE'), ('PHL','PH'), ('PCN','PN'), ('POL','PL'), ('PRT','PT'), ('PRI','PR'), ('QAT','QA'), ('REU','RE'), ('ROU','RO'), ('RUS','RU'), ('RWA','RW'), ('BLM','BL'), ('SHN','SH'), ('KNA','KN'), ('LCA','LC'), ('MAF','MF'), ('SPM','PM'), ('VCT','VC'), ('WSM','WS'), ('SMR','SM'), ('STP','ST'), ('SAU','SA'), ('SEN','SN'), ('SRB','RS'), ('SYC','SC'), ('SLE','SL'), ('SGP','SG'), ('SXM','SX'), ('SVK','SK'), ('SVN','SI'), ('SLB','SB'), ('SOM','SO'), ('ZAF','ZA'), ('SGS','GS'), ('SSD','SS'), ('ESP','ES'), ('LKA','LK'), ('SDN','SD'), ('SUR','SR'), ('SJM','SJ'), ('SWE','SE'), ('CHE','CH'), ('SYR','SY'), ('TWN','TW'), ('TJK','TJ'), ('TZA','TZ'), ('THA','TH'), ('TLS','TL'), ('TGO','TG'), ('TKL','TK'), ('TON','TO'), ('TTO','TT'), ('TUN','TN'), ('TUR','TR'), ('TKM','TM'), ('TCA','TC'), ('TUV','TV'), ('UGA','UG'), ('UKR','UA'), ('ARE','AE'), ('GBR','GB'), ('USA','US'), ('UMI','UM'), ('URY','UY'), ('UZB','UZ'), ('VUT','VU'), ('VEN','VE'), ('VNM','VN'), ('VGB','VG'), ('VIR','VI'), ('WLF','WF'), ('ESH','EH'), ('YEM','YE'), ('ZMB','ZM'), ('ZWE','ZW'))
UPDATE comparison_items c
SET geo_space_id = sp.id
FROM spaces sp
WHERE c.geo_space_id IS NULL
  AND sp.level = 'country' AND sp.deleted_at IS NULL
  AND (
    sp.geo_code = UPPER(btrim(c.title))
    OR lower(sp.name) = lower(btrim(c.title))
    OR sp.geo_code = (SELECT iso2_code FROM iso3 WHERE iso3_code = UPPER(btrim(c.title)))
  );

-- Pass 3: title -> city/state space -> country ancestor (parent chain,
-- up to 3 hops: city -> state -> country).
UPDATE comparison_items c
SET geo_space_id = anc.id
FROM spaces loc
JOIN spaces anc ON anc.level = 'country' AND anc.deleted_at IS NULL
  AND anc.id IN (
    SELECT loc.parent_id UNION
    SELECT s1.parent_id FROM spaces s1 WHERE s1.id = loc.parent_id UNION
    SELECT s2.parent_id FROM spaces s2
      JOIN spaces s1 ON s1.id = loc.parent_id AND s2.id = s1.parent_id
  )
WHERE c.geo_space_id IS NULL
  AND loc.deleted_at IS NULL
  AND loc.level <> 'country'
  AND lower(loc.name) = lower(btrim(c.title));
