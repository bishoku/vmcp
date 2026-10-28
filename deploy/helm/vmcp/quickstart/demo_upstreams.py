"""Twenty mock REST upstreams for the banking playground (WireMock-backed).

Each operation becomes an MCP tool. Inputs are JSON Schemas with descriptions,
patterns and enums; outputs are JSON Schemas inferred from the mock responses and
documented from FIELD_DOCS, so every mock response validates by construction.
Mock rules match on query parameters or JSON body fields and read from
demo_data, which keeps all services consistent with each other.
"""
import json

import demo_catalog as catalog
import demo_data as d


# ------------------------------------------------------------------ templating
class Raw(str):
    """A WireMock template inserted without JSON quotes (numbers)."""
    kind = 'number'


def jb(path):
    return "{{jsonPath request.body '$." + path + "'}}"


def qv(name):
    return '{{request.query.' + name + '}}'


NOW = '{{now}}'
UUID = "{{randomValue type='UUID'}}"


def days(n):
    return "{{now offset='" + str(n) + " days' format='yyyy-MM-dd'}}"


def rnd(n):
    return '{{randomValue length=' + str(n) + " type='NUMERIC'}}"


def _render(value):
    raws = []

    def walk(v):
        if isinstance(v, Raw):
            raws.append(str(v))
            return '__RAW_%d__' % (len(raws) - 1)
        if isinstance(v, dict):
            return {k: walk(x) for k, x in v.items()}
        if isinstance(v, list):
            return [walk(x) for x in v]
        return v
    text = json.dumps(walk(value))
    for i, raw in enumerate(raws):
        text = text.replace('"__RAW_%d__"' % i, raw)
    return text


JSON_HEADERS = {'Content-Type': 'application/json'}


def ok(body, status=200):
    return dict(status=status, headers=JSON_HEADERS, jsonBody=body)


def tpl(body, status=200):
    return dict(status=status, headers=JSON_HEADERS, body=_render(body), transformers=['response-template'])


def rule(name, response, priority=10, query=None, body=None):
    request = {}
    if query:
        request['queryParameters'] = query
    if body:
        request['bodyPatterns'] = body
    out = dict(name=name, priority=priority, response=response)
    if request:
        out['request'] = request
    return out


def eq(value):
    return {'equalTo': value}


def rx(pattern):
    return {'matches': pattern}


def body_eq(path, value):
    return {'matchesJsonPath': {'expression': '$.' + path, 'equalTo': value}}


def body_rx(path, pattern):
    return {'matchesJsonPath': {'expression': '$.' + path, 'matches': pattern}}


def not_found(entity, param):
    return rule('not-found', tpl({'error': 'NOT_FOUND', 'message': 'No ' + entity + ' found for ' + param + ' ' + qv(param)}, 404), priority=900)


def lookup(param, table, entity, generic=None):
    """One rule per known identifier, an optional generic rule, then 404."""
    rules = [rule(key.lower(), ok(body), query={param: eq(key)}) for key, body in table.items()]
    if generic:
        pattern, body = generic
        rules.append(rule('generic-' + param.lower(), tpl(body), priority=50, query={param: rx(pattern)}))
    rules.append(not_found(entity, param))
    return rules


# --------------------------------------------------------------------- schemas
def S(desc, **kw):
    return dict(type='string', description=desc, **kw)


def N(desc, **kw):
    return dict(type='number', description=desc, **kw)


def I(desc, **kw):
    return dict(type='integer', description=desc, **kw)


def B(desc):
    return dict(type='boolean', description=desc)


def obj(props, required=(), desc=None):
    out = dict(type='object', properties=props, required=list(required), additionalProperties=False)
    if desc:
        out['description'] = desc
    return out


def q(name, schema, required=True, default=None):
    out = {'name': name, 'in': 'query', 'required': required, 'schema': schema}
    if default is not None:
        out['defaultValue'] = default
    return out


ID = dict(
    customer=S('Meridian Bank customer number (CIF), format CUST-NNNNNN, e.g. CUST-100101. Resolve a name, email or company number with search_customers first.', pattern=r'^CUST-[0-9]{6}$'),
    account=S('Internal account identifier, format ACC-NNNNNNNN, e.g. ACC-10010101. List a customer\'s accounts with list_customer_accounts.', pattern=r'^ACC-[0-9]{8}$'),
    card=S('Card identifier, format CRD-NNNN, e.g. CRD-5008. Never pass a card number (PAN). List cards with list_customer_cards.', pattern=r'^CRD-[0-9]{4}$'),
    loan=S('Loan identifier, format LN-NNNNNN, e.g. LN-700101. List loans with list_customer_loans.', pattern=r'^LN-[0-9]{6}$'),
    product=S('Product code from the product catalogue, e.g. PL-STANDARD or MTG-FIX5. See list_banking_products.', pattern=r'^[A-Z]{2,4}(-[A-Z0-9]{1,10}){1,2}$'),
)
NAME_TEXT = r"^[A-Za-z0-9 .,'&()/-]{2,140}$"
REF_TEXT = r'^[A-Za-z0-9 ./-]{1,18}$'
FREE_TEXT = r"^[A-Za-z0-9 .,'&()/:;!?%+-]{10,500}$"
DATE = r'^20[0-9]{2}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$'

# Output field documentation, used for every inferred response property.
FIELD_DOCS = {
    'customerId': 'Customer number (CIF) of the customer the record belongs to.',
    'customerType': 'Legal form of the customer: INDIVIDUAL, LIMITED_COMPANY or PLC.',
    'fullName': 'Full legal name of the person or registered company name.',
    'segment': 'Customer segment: MASS_MARKET, YOUNG_ADULT, PREMIER, PRIVATE_BANKING, SME or CORPORATE.',
    'dateOfBirth': 'Date of birth (YYYY-MM-DD); null for companies.',
    'email': 'Primary email address on file.',
    'mobile': 'Primary phone number on file (E.164 style).',
    'address': 'Registered postal address.',
    'city': 'City of the registered address or branch.',
    'customerSince': 'Date the relationship started (YYYY-MM-DD).',
    'riskRating': 'Financial crime customer risk rating: LOW, MEDIUM or HIGH.',
    'kycStatus': 'Know-your-customer status: VERIFIED, REFRESH_REQUIRED or ENHANCED_DUE_DILIGENCE.',
    'homeBranchId': 'Branch that holds the relationship (see find_branches).',
    'relationshipManager': 'Assigned relationship manager or advisor; null for self-service segments.',
    'username': 'Bank colleague user name (matches the SSO user name).',
    'name': 'Display name.',
    'team': 'Owning bank team.',
    'title': 'Job title.',
    'employment': 'Occupation and employer as declared by the customer; null for companies.',
    'companyNumber': 'Companies House registration number (companies only).',
    'industry': 'Industry and SIC code (companies only).',
    'products': 'Identifiers of products held, for use with the account, card and loan tools.',
    'accountIds': 'Account identifiers held by the customer.',
    'cardIds': 'Card identifiers held by the customer.',
    'loanIds': 'Loan identifiers held by the customer.',
    'portfolioIds': 'Investment portfolio identifiers held by the customer.',
    'contactPreferences': 'Channel consent summary; use get_customer_contact_preferences for details.',
    'results': 'Matching customers, best match first. Empty when nothing matches.',
    'query': 'Search text as received.',
    'total': 'Number of records returned.',
    'accounts': 'Accounts held by the customer.',
    'accountId': 'Internal account identifier.',
    'holderName': 'Name of the account holder.',
    'productCode': 'Product code in the product catalogue.',
    'productName': 'Marketing name of the product.',
    'accountType': 'Account type: CURRENT, SAVINGS, CURRENCY, BUSINESS_CURRENT, BUSINESS_CURRENCY or CORPORATE.',
    'currency': 'ISO 4217 currency code.',
    'status': 'Lifecycle status of the record.',
    'sortCode': 'UK sort code (NN-NN-NN).',
    'accountNumber': 'UK account number (8 digits).',
    'iban': 'International Bank Account Number.',
    'bic': 'Bank Identifier Code (SWIFT) of the account-holding bank.',
    'ledgerBalance': 'Booked balance including all posted transactions.',
    'holds': 'Amount reserved by pending card authorisations or held payments.',
    'overdraftLimit': 'Arranged overdraft limit.',
    'availableBalance': 'Funds available to spend: ledger balance minus holds plus arranged overdraft.',
    'interestRate': 'Annual interest rate in percent.',
    'openedOn': 'Date the account was opened.',
    'branchId': 'Branch identifier.',
    'restrictions': 'Active restrictions on the account, e.g. INTERNATIONAL_PAYMENTS_BLOCKED_PENDING_KYC_REFRESH.',
    'asOf': 'Timestamp the figures are valid for (ISO 8601, UTC).',
    'transactions': 'Transactions, newest first.',
    'transactionId': 'Unique transaction identifier.',
    'bookingDate': 'Date the transaction was booked.',
    'valueDate': 'Date the funds were credited or debited for interest purposes.',
    'description': 'Transaction or record description.',
    'counterparty': 'Payer or payee name.',
    'amount': 'Amount in the stated currency (always positive; see direction).',
    'direction': 'CREDIT (money in) or DEBIT (money out).',
    'category': 'Spending or record category.',
    'channel': 'Channel or payment scheme used.',
    'reference': 'Payment reference or related identifier.',
    'runningBalance': 'Ledger balance after this transaction; null if the transaction is not booked.',
    'statementId': 'Statement identifier.',
    'period': 'Statement period (YYYY-MM).',
    'openingBalance': 'Balance at the start of the period.',
    'closingBalance': 'Balance at the end of the period.',
    'totalCredits': 'Sum of booked credits in the period.',
    'totalDebits': 'Sum of booked debits in the period.',
    'transactionCount': 'Number of booked transactions in the period.',
    'documentUrl': 'Secure link to the PDF statement (valid 15 minutes).',
    'emailConsent': 'Consent to service messages by email.',
    'smsConsent': 'Consent to service messages by SMS.',
    'pushConsent': 'Consent to mobile push notifications.',
    'postConsent': 'Consent to postal letters.',
    'marketingOptIn': 'Whether the customer agreed to receive marketing.',
    'preferredChannel': 'Customer\'s preferred servicing channel.',
    'preferredLanguage': 'Preferred language (BCP 47).',
    'vulnerabilityIndicators': 'Recorded vulnerability indicators that require adapted treatment, e.g. FINANCIAL_DIFFICULTY.',
    'quietHours': 'Hours during which outbound contact must not be made (local time).',
    'changeRequestId': 'Identifier of the change request.',
    'verification': 'How the change will be verified before it takes effect.',
    'requestedAt': 'Time the request was received.',
    'payees': 'Saved payees, most recently added first.',
    'payeeId': 'Saved payee identifier.',
    'addedOn': 'Date the payee was added.',
    'result': 'Outcome of the check.',
    'matchedName': 'Name held by the receiving bank, returned only on CLOSE_MATCH.',
    'advice': 'Recommended next step to show to the customer.',
    'paymentId': 'Payment identifier.',
    'sourceAccountId': 'Account the payment is debited from.',
    'beneficiaryName': 'Name of the beneficiary.',
    'scheme': 'Payment scheme: FASTER_PAYMENTS, BACS or CHAPS.',
    'createdAt': 'Time the record was created (ISO 8601, UTC).',
    'settledAt': 'Time the payment settled; null if not settled.',
    'rejectionReason': 'Reason the payment was rejected; null if not rejected.',
    'estimatedSettlement': 'Expected settlement time for the scheme.',
    'standingOrders': 'Standing orders on the account.',
    'standingOrderId': 'Standing order identifier.',
    'payeeName': 'Name of the payee.',
    'frequency': 'Payment frequency.',
    'nextPaymentDate': 'Date of the next scheduled payment.',
    'lastPaymentStatus': 'Outcome of the most recent execution.',
    'effectiveFrom': 'Date the change takes effect.',
    'quoteId': 'FX quote identifier; pass to initiate_international_payment within its validity.',
    'sellCurrency': 'Currency debited.',
    'buyCurrency': 'Currency delivered to the beneficiary.',
    'sellAmount': 'Amount debited in the sell currency.',
    'customerRate': 'Customer exchange rate (buy currency units per one sell currency unit) including margin.',
    'midMarketRate': 'Mid-market reference rate.',
    'marginPercent': 'Bank FX margin applied, in percent.',
    'estimatedBuyAmount': 'Estimated amount delivered in the buy currency, before correspondent fees.',
    'fee': 'Payment fee charged in the sell currency.',
    'expiresAt': 'Time the quote expires.',
    'uetr': 'SWIFT gpi Unique End-to-end Transaction Reference (UUID).',
    'statusDescription': 'Human-readable status explanation.',
    'trackingInfo': 'How to track the payment.',
    'debtorAccountId': 'Account debited.',
    'beneficiaryBic': 'BIC of the beneficiary bank.',
    'initiatedAt': 'Time the payment was sent.',
    'completedAt': 'Time the beneficiary was credited; null while in progress.',
    'totalFees': 'Total correspondent and beneficiary bank fees deducted so far.',
    'hops': 'Banks the payment passed through, in order.',
    'bank': 'Bank name.',
    'timestamp': 'Event time (ISO 8601, UTC).',
    'cards': 'Cards held by the customer.',
    'cardId': 'Card identifier.',
    'cardType': 'DEBIT or CREDIT.',
    'maskedPan': 'Masked card number; full numbers are never returned.',
    'linkedAccountId': 'Account a debit card draws on; null for credit cards.',
    'expiryDate': 'Expiry date (YYYY-MM or YYYY-MM-DD).',
    'creditLimit': 'Credit limit; null for debit cards.',
    'currentBalance': 'Outstanding balance; null for debit cards.',
    'availableCredit': 'Remaining credit; null for debit cards.',
    'dailyPurchaseLimit': 'Maximum card purchases per day.',
    'dailyAtmLimit': 'Maximum ATM withdrawals per day.',
    'onlinePaymentsEnabled': 'Whether card-not-present (online) payments are allowed.',
    'contactlessEnabled': 'Whether contactless payments are allowed.',
    'openFraudAlert': 'Whether an unresolved fraud alert exists on the card.',
    'statusReason': 'Reason for the current card status.',
    'replacementCard': 'Replacement card order; null when no replacement is needed.',
    'deliveryEstimate': 'Expected delivery date.',
    'reason': 'Reason supplied or applied.',
    'message': 'Explanation for the agent and customer.',
    'updatedAt': 'Time the change was applied.',
    'merchantName': 'Merchant name as presented by the card scheme.',
    'merchantCategoryCode': 'ISO 18245 merchant category code.',
    'merchantCountry': 'Merchant country (ISO 3166 alpha-2).',
    'cardPresent': 'Whether the card was physically present.',
    'disputeReference': 'Dispute opened for this transaction, if any.',
    'disputeId': 'Dispute case identifier.',
    'reasonCode': 'Dispute reason code.',
    'openedOn': 'Date the case was opened.',
    'provisionalCreditIssued': 'Whether a temporary credit was applied to the customer.',
    'provisionalCreditAmount': 'Temporary credit amount.',
    'schemeDeadline': 'Card scheme deadline for the next action.',
    'assignedAnalyst': 'Colleague handling the case.',
    'timeline': 'Chronological case events.',
    'nextStep': 'What happens next.',
    'alerts': 'Alerts, newest first.',
    'alertId': 'Alert identifier.',
    'riskScore': 'Model risk score from 0 (genuine) to 1 (fraudulent).',
    'severity': 'Alert severity: LOW, MEDIUM or HIGH.',
    'rule': 'Detection rule or scenario that triggered the alert.',
    'customerContacted': 'Whether the customer has been contacted about the alert.',
    'recommendedAction': 'Recommended handling.',
    'score': 'Score value.',
    'decision': 'Recommended decision.',
    'reasons': 'Main factors behind the score or decision.',
    'modelVersion': 'Scoring model version.',
    'outcome': 'Recorded outcome.',
    'nextActions': 'Follow-up actions created by the system or required from the agent.',
    'resolvedAt': 'Time the alert was resolved.',
    'screeningId': 'Screening request identifier.',
    'screenedName': 'Name that was screened.',
    'matchStatus': 'CLEAR, POTENTIAL_MATCH or PEP_MATCH.',
    'hits': 'Watch-list entries that matched.',
    'list': 'Watch list name.',
    'entityName': 'Name of the listed entity.',
    'matchScore': 'Fuzzy match score from 0 to 1.',
    'listingReason': 'Reason the entity is listed.',
    'screenedAt': 'Time of screening.',
    'caseId': 'Case identifier.',
    'alertType': 'Monitoring scenario.',
    'priority': 'Case priority.',
    'dueDate': 'Date the case or document is due.',
    'assignedInvestigator': 'Investigator assigned to the case.',
    'summary': 'Case summary.',
    'linkedTransactions': 'Related transaction identifiers.',
    'linkedPayments': 'Related payment UETRs.',
    'screeningHits': 'Sanctions or PEP screening hits linked to the case.',
    'sarDecision': 'Suspicious Activity Report decision: PENDING, SUBMITTED or NOT_REQUIRED.',
    'nextSteps': 'Outstanding investigation steps.',
    'lastReviewDate': 'Date of the last completed review.',
    'nextReviewDue': 'Date the next review is due.',
    'documents': 'Documents on file.',
    'type': 'Type of the item.',
    'outstandingRequirements': 'Items still required from the customer.',
    'openCaseId': 'Open KYC case, if any.',
    'requiredDocuments': 'Documents to collect.',
    'enhancedDueDiligence': 'Additional documents required for high-risk customers.',
    'existingCaseId': 'Case already in progress.',
    'bureau': 'Credit reference agency.',
    'band': 'Score band.',
    'scoreScale': 'Score scale.',
    'activeAccounts': 'Open credit agreements reported.',
    'missedPayments12m': 'Missed payments reported in the last 12 months.',
    'defaults': 'Defaults recorded.',
    'countyCourtJudgments': 'County Court Judgments recorded.',
    'hardSearches6m': 'Hard credit searches in the last 6 months.',
    'creditUtilisation': 'Revolving credit utilisation in percent.',
    'electoralRoll': 'Electoral roll registration at current address; null for companies.',
    'reportDate': 'Date the report was retrieved.',
    'grade': 'Internal risk grade from 1 (lowest risk) to 10.',
    'probabilityOfDefault12m': '12-month probability of default in percent.',
    'monthlyDisposableIncome': 'Estimated monthly disposable income in GBP; null where not assessed.',
    'maxRecommendedUnsecuredExposure': 'Maximum recommended total unsecured exposure in GBP.',
    'policyFlags': 'Credit policy flags that require attention.',
    'gradedAt': 'Time the grade was calculated.',
    'representativeApr': 'Representative APR in percent.',
    'rateType': 'FIXED, VARIABLE, TRACKER or NONE.',
    'minAmount': 'Minimum amount.',
    'maxAmount': 'Maximum amount.',
    'minTermMonths': 'Minimum term in months.',
    'maxTermMonths': 'Maximum term in months.',
    'requestedAmount': 'Requested amount.',
    'termMonths': 'Term in months.',
    'monthlyPaymentPer1000': 'Monthly payment per 1,000 GBP borrowed at the representative rate, for common terms.',
    'fees': 'Fees that apply.',
    'disclaimer': 'Regulatory disclaimer.',
    'applicationId': 'Loan application identifier.',
    'purpose': 'Loan purpose.',
    'submittedAt': 'Time the application was submitted.',
    'underwriter': 'Underwriter assigned.',
    'offer': 'Offer terms; null until approved.',
    'monthlyPayment': 'Contractual monthly payment.',
    'arrangementFee': 'Arrangement fee.',
    'offerExpiresOn': 'Date the offer expires.',
    'declineReasons': 'Reasons for a decline.',
    'loans': 'Loans held by the customer.',
    'loanId': 'Loan identifier.',
    'loanType': 'MORTGAGE, PERSONAL_LOAN, BUSINESS_TERM_LOAN or REVOLVING_CREDIT_FACILITY.',
    'originalAmount': 'Amount originally advanced or facility limit.',
    'outstandingPrincipal': 'Principal outstanding (drawn amount for facilities).',
    'fixedUntil': 'End of the fixed-rate period; null if not fixed.',
    'startDate': 'Start date.',
    'maturityDate': 'Final repayment date.',
    'daysPastDue': 'Days since the oldest missed payment; 0 when up to date.',
    'arrearsAmount': 'Overdue amount.',
    'collateral': 'Security held; null if unsecured.',
    'loanToValue': 'Loan-to-value in percent (mortgages).',
    'repaymentAccountId': 'Account repayments are collected from.',
    'covenants': 'Financial covenants and latest test results.',
    'installments': 'Upcoming installments.',
    'installmentNumber': 'Installment sequence number.',
    'totalPayment': 'Total installment amount.',
    'principal': 'Principal portion.',
    'interest': 'Interest portion.',
    'accruedInterest': 'Interest accrued since the last payment.',
    'earlyRepaymentCharge': 'Early repayment charge.',
    'chargeBasis': 'How the early repayment charge is calculated.',
    'settlementAmount': 'Total amount to settle the loan in full.',
    'validUntil': 'Date until which the figure is valid.',
    'paymentInstructions': 'How to pay the settlement amount.',
    'stage': 'Collections stage.',
    'missedPayments': 'Number of missed payments.',
    'assignedCollector': 'Collections colleague assigned.',
    'lastContact': 'Most recent contact attempt.',
    'date': 'Date.',
    'promiseToPay': 'Active promise to pay; null if none.',
    'recommendedTreatment': 'Recommended forbearance or treatment.',
    'arrangementId': 'Payment arrangement identifier.',
    'monthlyAmount': 'Agreed monthly amount.',
    'numberOfMonths': 'Duration of the arrangement in months.',
    'firstPaymentDate': 'First payment date.',
    'arrearsClearedBy': 'Whether the arrangement clears the arrears within its term.',
    'baseCurrency': 'Base currency of the rates.',
    'rates': 'Rates against the base currency.',
    'mid': 'Mid-market rate (quote currency units per one base currency unit).',
    'bankBuys': 'Rate at which the bank buys the quote currency.',
    'bankSells': 'Rate at which the bank sells the quote currency.',
    'change24hPercent': '24-hour change in percent.',
    'curve': 'Curve name.',
    'points': 'Curve points by tenor.',
    'tenor': 'Tenor (O/N overnight, M months, Y years).',
    'ratePercent': 'Rate in percent.',
    'source': 'Market data source.',
    'liquidityCoverageRatio': 'Liquidity Coverage Ratio in percent.',
    'lcrRegulatoryMinimum': 'Regulatory LCR minimum in percent.',
    'lcrInternalTrigger': 'Internal early-warning LCR trigger in percent.',
    'netStableFundingRatio': 'Net Stable Funding Ratio in percent.',
    'highQualityLiquidAssets': 'Stock of high-quality liquid assets.',
    'netCashOutflows30d': 'Stressed net cash outflows over 30 days.',
    'reportingCurrency': 'Currency of aggregate figures.',
    'positions': 'Positions by currency.',
    'cashAtCentralBank': 'Reserves held at the central bank.',
    'netPosition30d': 'Projected 30-day net cash position.',
    'limitUtilisationPercent': 'Utilisation of the internal limit in percent.',
    'lettersOfCredit': 'Letters of credit.',
    'lcReference': 'Letter of credit reference.',
    'applicant': 'Party that requested the instrument.',
    'beneficiary': 'Party that benefits from the instrument.',
    'issuingBank': 'Issuing bank and BIC.',
    'advisingBank': 'Advising or confirming bank and BIC.',
    'issueDate': 'Issue date.',
    'latestShipmentDate': 'Latest permitted shipment date.',
    'incoterm': 'Incoterms 2020 rule and place.',
    'portOfLoading': 'Port of loading.',
    'portOfDischarge': 'Port of discharge.',
    'goods': 'Goods description.',
    'documentsRequired': 'Documents required for presentation.',
    'discrepancies': 'Discrepancies found in presented documents.',
    'utilisedAmount': 'Amount already drawn.',
    'settlementUetr': 'UETR of the settlement payment, if paid.',
    'guarantees': 'Bank guarantees.',
    'guaranteeId': 'Guarantee identifier.',
    'underlyingContract': 'Underlying commercial contract.',
    'claimsMade': 'Number of claims made.',
    'portfolios': 'Investment portfolios.',
    'portfolioId': 'Portfolio identifier.',
    'mandateType': 'DISCRETIONARY or ADVISORY.',
    'marketValue': 'Current market value.',
    'ytdReturnPercent': 'Year-to-date return in percent.',
    'oneYearReturnPercent': 'One-year return in percent.',
    'riskLevel': 'Risk level from 1 (cautious) to 7 (speculative).',
    'inceptionDate': 'Portfolio inception date.',
    'manager': 'Portfolio manager or advisor.',
    'assetAllocation': 'Allocation by asset class in percent.',
    'equities': 'Equities in percent.',
    'fixedIncome': 'Fixed income in percent.',
    'alternatives': 'Alternatives in percent.',
    'cash': 'Cash in percent.',
    'totalValue': 'Total value of all portfolios.',
    'holdings': 'Holdings, largest first.',
    'instrumentId': 'Instrument identifier (ISIN or cash code).',
    'assetClass': 'Asset class.',
    'weightPercent': 'Portfolio weight in percent.',
    'profile': 'Risk profile name.',
    'scale': 'Risk scale definition.',
    'assessedOn': 'Date of the suitability assessment.',
    'capacityForLoss': 'Ability to absorb losses: LOW, MEDIUM or HIGH.',
    'knowledgeAndExperience': 'Investment knowledge and experience.',
    'investmentHorizonYears': 'Investment horizon in years.',
    'objectives': 'Investment objectives.',
    'eligibility': 'Eligibility criteria.',
    'features': 'Key features.',
    'monthlyFee': 'Monthly fee in GBP.',
    'overdraftApr': 'Arranged overdraft APR in percent (0 when not applicable).',
    'savingsProducts': 'Savings products and rates.',
    'aer': 'Annual Equivalent Rate in percent.',
    'branches': 'Matching branches.',
    'openingHours': 'Opening hours.',
    'services': 'Services offered.',
    'accessible': 'Step-free access available.',
    'phone': 'Telephone number.',
    'atms': 'Matching ATMs.',
    'atmId': 'ATM identifier.',
    'location': 'Location description.',
    'postcode': 'Postcode.',
    'available24h': 'Available 24 hours a day.',
    'depositEnabled': 'Accepts cash and cheque deposits.',
    'appointmentId': 'Appointment identifier.',
    'topic': 'Appointment topic.',
    'timeSlot': 'Time slot.',
    'advisor': 'Advisor allocation.',
    'messageId': 'Message identifier.',
    'templateId': 'Notification template identifier.',
    'sentAt': 'Time the message was sent.',
    'customerResponse': 'Customer response, if the template expects one.',
    'notifications': 'Messages, newest first.',
    'templates': 'Available templates.',
    'channels': 'Channels the template supports.',
    'parameters': 'Template parameters that must be supplied.',
    'complaints': 'Complaints, newest first.',
    'complaintId': 'Complaint identifier.',
    'relatedReference': 'Related record (dispute, account or payment).',
    'handler': 'Complaint handler.',
    'acknowledgedOn': 'Date the complaint was acknowledged.',
    'finalResponseDue': 'Regulatory deadline for the final response (8 weeks).',
    'redressPaid': 'Redress or refund paid.',
    'eligibleForOmbudsman': 'Whether the customer can refer the complaint to the Financial Ombudsman Service after the deadline.',
    'error': 'Error code.',
}


def _kind(v):
    if isinstance(v, Raw):
        return v.kind
    if v is None:
        return 'null'
    if isinstance(v, bool):
        return 'boolean'
    if isinstance(v, int):
        return 'integer'
    if isinstance(v, float):
        return 'number'
    if isinstance(v, str):
        return 'string'
    if isinstance(v, list):
        return 'array'
    return 'object'


def infer(samples, name=None, docs=None):
    """Merge example values into a JSON Schema; descriptions come from docs."""
    docs = docs or {}
    kinds = sorted({_kind(s) for s in samples})
    if 'number' in kinds and 'integer' in kinds:
        kinds.remove('integer')
    schema = {'type': kinds[0] if len(kinds) == 1 else kinds}
    if name is not None:
        description = docs.get(name) or FIELD_DOCS.get(name)
        if description:
            schema['description'] = description
    objects = [s for s in samples if isinstance(s, dict)]
    if objects:
        keys = []
        for o in objects:
            keys += [k for k in o if k not in keys]
        schema['properties'] = {k: infer([o[k] for o in objects if k in o], k, docs) for k in keys}
        schema['required'] = [k for k in keys if all(k in o for o in objects)]
    arrays = [s for s in samples if isinstance(s, list)]
    if arrays:
        items = [x for a in arrays for x in a]
        schema['items'] = infer(items, None, docs) if items else {'type': 'string'}
    return schema


def _bodies(rules):
    out = []
    for r in rules:
        response = r['response']
        if response['status'] >= 300:
            continue
        out.append(response['jsonBody'] if 'jsonBody' in response else r['_sample'])
    return out


def op(name, method, path, description, rules, params=(), body=None, docs=None, summary=None, status_docs=None):
    """Build a REST operation; the response schema is inferred from the 2xx mock bodies."""
    docs = dict(docs or {})
    if status_docs:
        docs['status'] = status_docs
    response = infer(_bodies(rules), None, docs)
    response['description'] = summary or description.split('. ')[0].rstrip('.') + '.'
    clean = [{k: v for k, v in r.items() if k != '_sample'} for r in rules]
    out = dict(name=name, description=description, enabled=True, method=method, path=path, parameters=list(params),
               responseSchema=response, readOnly=method == 'GET', mockRules=clean)
    if body is not None:
        out['bodySchema'] = body
    return out


def trule(name, body, status=200, priority=10, query=None, match=None):
    """Templated rule. Keeps the unrendered body as the schema sample."""
    r = rule(name, tpl(body, status), priority, query, match)
    r['_sample'] = body
    return r


def tlookup(param, table, entity, generic):
    rules = lookup(param, table, entity)
    pattern, body = generic
    rules.insert(-1, trule('generic-' + param.lower(), body, priority=50, query={param: rx(pattern)}))
    return rules


# ------------------------------------------------------------------- helpers
USERS_BY_REF = {key: catalog.staff(team, pos) for key, (team, pos) in d.STAFF_REFS.items()}


def staff(key):
    return d.staff_label(key, USERS_BY_REF)


CUSTOMER_IDS = [c['customerId'] for c in d.CUSTOMERS]


def per_customer(fn):
    return {cid: fn(cid) for cid in CUSTOMER_IDS}


def public_customer(c):
    keys = ['customerId', 'customerType', 'fullName', 'segment', 'dateOfBirth', 'email', 'mobile', 'address', 'city', 'customerSince',
            'status', 'riskRating', 'kycStatus', 'homeBranchId', 'employment', 'companyNumber', 'industry']
    out = {k: c.get(k) for k in keys}
    out['relationshipManager'] = staff(c['relationshipManager'])
    return out


def account_summary(a):
    keys = ['accountId', 'iban', 'productCode', 'productName', 'accountType', 'currency', 'status', 'availableBalance', 'ledgerBalance', 'openedOn']
    return {k: a[k] for k in keys}


def card_public(c):
    out = {k: v for k, v in c.items() if k not in ('statusReason',)}
    out['statusReason'] = c.get('statusReason')
    return out


def with_staff(row, *fields):
    out = dict(row)
    for f in fields:
        out[f] = staff(row[f])
    return out


UPSTREAMS = []


def upstream(slug, name, owner, description, operations):
    UPSTREAMS.append(dict(slug=slug, name=name, ownerTeam=owner, description=description, operations=operations))


# ======================================================== 1. core banking
upstream('core-banking-accounts', 'Core Banking - Accounts & Ledger', 'core-banking-platform',
         'System of record for deposit accounts: account master data, balances, holds, booked and pending transactions, and monthly statements.', [
    op('list_customer_accounts', 'GET', '/core/v2/accounts',
       'List all deposit accounts held by a customer (current, savings, currency, business and corporate accounts) with balances. '
       'Use this first to discover account IDs for balance, transaction, statement and payment tools. Returns HTTP 404 for unknown customers.',
       lookup('customerId', per_customer(lambda cid: dict(customerId=cid, accounts=[account_summary(a) for a in d.by_customer(d.ACCOUNTS, cid)])), 'customer'),
       params=[q('customerId', ID['customer'])], status_docs='Account status: ACTIVE, DORMANT, BLOCKED or CLOSED.'),
    op('get_account_details', 'GET', '/core/v2/accounts/details',
       'Get full master data for one account: holder, product, sort code, account number, IBAN/BIC, overdraft limit, interest rate, home branch and any restrictions. '
       'Use for payment instructions or to explain why an account is restricted.',
       lookup('accountId', {a['accountId']: {k: v for k, v in a.items()} for a in d.ACCOUNTS}, 'account'),
       params=[q('accountId', ID['account'])], status_docs='Account status: ACTIVE, DORMANT, BLOCKED or CLOSED.'),
    op('get_account_balance', 'GET', '/core/v2/accounts/balance',
       'Get the real-time balance of one account: ledger balance, holds from pending card authorisations or held payments, arranged overdraft and available balance. '
       'Available balance = ledger - holds + overdraft limit. Call before initiating any payment.',
       lookup('accountId', {a['accountId']: dict(accountId=a['accountId'], currency=a['currency'], ledgerBalance=a['ledgerBalance'], holds=a['holds'],
                                                 overdraftLimit=a['overdraftLimit'], availableBalance=a['availableBalance'], asOf=d.AS_OF) for a in d.ACCOUNTS}, 'account'),
       params=[q('accountId', ID['account'])]),
    op('list_account_transactions', 'GET', '/core/v2/accounts/transactions',
       'List recent transactions on an account, newest first, including pending, rejected and returned items. Each booked item carries the running balance. '
       'Transaction IDs can be passed to get_transaction_risk_score.',
       lookup('accountId', {a['accountId']: dict(accountId=a['accountId'], currency=a['currency'], transactions=d.account_transactions(a['accountId'])) for a in d.ACCOUNTS}, 'account'),
       params=[q('accountId', ID['account']), q('limit', I('Maximum number of transactions to return (1-100).', minimum=1, maximum=100), required=False, default=20)],
       status_docs='BOOKED, PENDING (authorised, not yet posted), REJECTED (not executed) or RETURNED (collected then returned unpaid).'),
    op('get_account_statement', 'GET', '/core/v2/accounts/statement',
       'Get the statement summary for an account and month: opening and closing balance, total credits and debits, and a secure PDF link. '
       'Use for reconciliation, cash-flow analysis and audit evidence.',
       [trule(a['accountId'].lower(), dict(statementId='STM-' + a['accountId'][-8:] + '-' + qv('period'), accountId=a['accountId'], period=qv('period'),
                                           currency=a['currency'], **d.account_statement(a['accountId']),
                                           documentUrl='https://documents.meridianbank.example/statements/' + a['accountId'] + '/' + qv('period') + '.pdf'),
              query={'accountId': eq(a['accountId'])}) for a in d.ACCOUNTS] + [not_found('account', 'accountId')],
       params=[q('accountId', ID['account']), q('period', S('Statement month in YYYY-MM format, e.g. 2026-10.', pattern=r'^20[0-9]{2}-(0[1-9]|1[0-2])$'))]),
])

# ======================================================= 2. customer master
def profile(cid):
    c = d.CUSTOMER[cid]
    out = public_customer(c)
    out['products'] = dict(accountIds=[a['accountId'] for a in d.by_customer(d.ACCOUNTS, cid)], cardIds=[x['cardId'] for x in d.by_customer(d.CARDS, cid)],
                           loanIds=[x['loanId'] for x in d.by_customer(d.LOANS, cid)], portfolioIds=[x['portfolioId'] for x in d.by_customer(d.PORTFOLIOS, cid)])
    prefs = d.CONTACT_PREFERENCES[cid]
    out['contactPreferences'] = dict(emailConsent=prefs['email'], smsConsent=prefs['sms'], pushConsent=prefs['push'], postConsent=prefs['post'],
                                     marketingOptIn=prefs['marketingOptIn'], vulnerabilityIndicators=prefs['vulnerabilityIndicators'])
    return out


def contact_preferences(cid):
    p = d.CONTACT_PREFERENCES[cid]
    return dict(customerId=cid, emailConsent=p['email'], smsConsent=p['sms'], pushConsent=p['push'], postConsent=p['post'], marketingOptIn=p['marketingOptIn'],
                preferredChannel=p['preferredChannel'], preferredLanguage='en-GB', quietHours='21:00-08:00', vulnerabilityIndicators=p['vulnerabilityIndicators'])


def search_row(c):
    return dict(customerId=c['customerId'], fullName=c['fullName'], customerType=c['customerType'], segment=c['segment'], city=c['city'], status=c['status'])


_search_rules = []
for _c in d.CUSTOMERS:
    _terms = '|'.join(_c['searchTerms'] + [_c['customerId'].lower(), _c['email'].split('@')[0].lower()])
    _search_rules.append(trule('match-' + _c['customerId'].lower(), dict(query=qv('query'), total=1, results=[search_row(_c)]), query={'query': rx('(?i).*(' + _terms + ').*')}))
_search_rules.append(trule('no-match', dict(query=qv('query'), total=0, results=[]), priority=500))

upstream('customer-master', 'Customer Master Data (CIF)', 'customer-data-management',
         'Customer information file: identity, segment, risk rating, relationship manager, product holdings, contact data and channel consents.', [
    op('search_customers', 'GET', '/cif/v1/customers/search',
       'Find customers by name, surname, email, customer number or company registration number (case-insensitive, partial match). '
       'Returns customer IDs needed by all other tools. If several people could match, confirm identity before proceeding. Returns an empty list when nothing matches.',
       _search_rules, params=[q('query', S('Search text: a name, surname, email address, customer number or company number, e.g. "Ava Thompson" or "09876543".', minLength=2, maxLength=100))]),
    op('get_customer_profile', 'GET', '/cif/v1/customers/profile',
       'Get the full customer profile: identity, segment, KYC status, financial crime risk rating, home branch, relationship manager, product identifiers and a consent summary. '
       'Use as the starting point for any customer conversation.',
       lookup('customerId', per_customer(profile), 'customer'), params=[q('customerId', ID['customer'])],
       status_docs='Customer status: ACTIVE, DORMANT, DECEASED or EXITED.'),
    op('get_customer_contact_preferences', 'GET', '/cif/v1/customers/contact-preferences',
       'Get channel consents (email, SMS, push, post), marketing opt-in, preferred channel and language, quiet hours and vulnerability indicators. '
       'Check before any outbound contact or product recommendation.',
       lookup('customerId', per_customer(contact_preferences), 'customer'),
       params=[q('customerId', ID['customer'])]),
    op('update_customer_contact_details', 'PATCH', '/cif/v1/customers/contact-details',
       'Request a change of email address, mobile number or postal address. The change is not applied immediately: a one-time passcode is sent to the existing registered mobile number '
       'and the change takes effect after verification. Supply only the fields that change.',
       [trule('accepted', dict(changeRequestId='CHG-' + rnd(8), customerId=jb('customerId'), status='PENDING_VERIFICATION',
                               verification='One-time passcode sent to the mobile number already on file; change applies after verification.', requestedAt=NOW), status=202)],
       body=obj(dict(customerId=ID['customer'], email=S('New email address.', format='email', maxLength=254),
                     mobile=S('New mobile number in international format, e.g. +44 7700 900123.', pattern=r'^\+[0-9 ]{8,20}$'),
                     address=S('New full postal address including postcode.', pattern=NAME_TEXT)), ['customerId'],
                'Contact detail change request. Include at least one of email, mobile or address.'),
       status_docs='PENDING_VERIFICATION until the customer confirms the passcode.'),
])

# ======================================================== 3. payments hub
_cop_rules = []
for _p in d.PAYEES:
    _full = _p['name'].lower().replace(' ', r'\s+')
    _cop_rules.append(rule(_p['payeeId'].lower() + '-match', ok(dict(result='MATCH', matchedName=None, advice='Name matches the account holder. Safe to proceed.')),
                           query={'accountNumber': eq(_p['accountNumber']), 'payeeName': rx('(?i)^\\s*' + _full + '\\s*$')}))
    _cop_rules.append(rule(_p['payeeId'].lower() + '-close', ok(dict(result='CLOSE_MATCH', matchedName=_p['name'], advice='Name is similar but not exact. Show the matched name to the customer and ask them to confirm or correct it.')),
                           priority=20, query={'accountNumber': eq(_p['accountNumber']), 'payeeName': rx('(?i).*' + _p['closeMatch'] + '.*')}))
_cop_rules.append(rule('no-match', ok(dict(result='NO_MATCH', matchedName=None, advice='The name does not match the account. Warn the customer about possible fraud and do not proceed without re-checking the details.')), priority=500))

_payment_body = dict(paymentId='PAY-2026' + rnd(6), status='ACCEPTED', sourceAccountId=jb('sourceAccountId'), beneficiaryName=jb('beneficiaryName'),
                     amount=Raw(jb('amount')), currency='GBP', scheme=jb('scheme'), reference=jb('reference'), createdAt=NOW, rejectionReason=None)
_settle = {'FASTER_PAYMENTS': 'Usually within 2 minutes, at most 2 hours', 'BACS': 'Third working day (' + days(3) + ')', 'CHAPS': 'Same working day if submitted before 17:00'}

upstream('payments-hub', 'Domestic Payments Hub', 'payments-engineering',
         'UK domestic payments: saved payees, Confirmation of Payee, Faster Payments, BACS and CHAPS initiation, payment status and standing orders.', [
    op('list_saved_payees', 'GET', '/payments/v3/payees',
       'List the customer\'s saved payees with sort code, account number and default reference. Saved payees were verified with Confirmation of Payee when added.',
       lookup('customerId', per_customer(lambda cid: dict(customerId=cid, payees=[{k: v for k, v in p.items() if k not in ('customerId', 'closeMatch')} for p in d.by_customer(d.PAYEES, cid)])), 'customer'),
       params=[q('customerId', ID['customer'])]),
    op('validate_payee_name', 'GET', '/payments/v3/confirmation-of-payee',
       'Confirmation of Payee (CoP): check that a payee name matches the name on the receiving account before paying a new payee. '
       'Results: MATCH (proceed), CLOSE_MATCH (show matchedName and ask the customer to confirm), NO_MATCH (warn about authorised push payment fraud; do not proceed).',
       _cop_rules, params=[q('sortCode', S('Receiving bank sort code, format NN-NN-NN, e.g. 40-11-62.', pattern=r'^[0-9]{2}-[0-9]{2}-[0-9]{2}$')),
                           q('accountNumber', S('Receiving account number, 8 digits.', pattern=r'^[0-9]{8}$')),
                           q('payeeName', S('Name of the person or business as entered by the customer.', pattern=NAME_TEXT))],
       docs={'result': 'Confirmation of Payee outcome: MATCH, CLOSE_MATCH or NO_MATCH.'}),
    op('initiate_domestic_payment', 'POST', '/payments/v3/domestic-payments',
       'Send a UK domestic payment from a customer account. This moves money and cannot be recalled once accepted. '
       'Prerequisites: confirm amount, payee and reference with the customer; check get_account_balance; run validate_payee_name for new payees. '
       'Faster Payments up to 25,000 GBP settle in minutes, BACS in 3 working days, CHAPS same day for high values. Rejected payments return status REJECTED with a reason.',
       [trule('insufficient-funds', dict(paymentId='PAY-2026' + rnd(6), status='REJECTED', sourceAccountId=jb('sourceAccountId'), beneficiaryName=jb('beneficiaryName'),
                                         amount=Raw(jb('amount')), currency='GBP', scheme=jb('scheme'), reference=jb('reference'), createdAt=NOW,
                                         rejectionReason='INSUFFICIENT_FUNDS', estimatedSettlement=None), status=201, priority=5,
              match=[body_eq('sourceAccountId', 'ACC-10010201')])] +
       [trule('accepted-' + scheme.lower().replace('_', '-'), dict(_payment_body, estimatedSettlement=text), status=201, priority=20,
              match=[body_eq('scheme', scheme)]) for scheme, text in _settle.items()],
       body=obj(dict(sourceAccountId=ID['account'], beneficiaryName=S('Payee name exactly as validated with validate_payee_name.', pattern=NAME_TEXT),
                     sortCode=S('Payee sort code, NN-NN-NN.', pattern=r'^[0-9]{2}-[0-9]{2}-[0-9]{2}$'),
                     accountNumber=S('Payee account number, 8 digits.', pattern=r'^[0-9]{8}$'),
                     amount=N('Amount in GBP, maximum 2 decimal places.', exclusiveMinimum=0, maximum=1000000, multipleOf=0.01),
                     reference=S('Payment reference shown to the payee, up to 18 characters (letters, digits, space . / -).', pattern=REF_TEXT),
                     scheme=S('Payment scheme. Use FASTER_PAYMENTS by default, CHAPS for urgent high-value payments, BACS for non-urgent bulk payments.', enum=['FASTER_PAYMENTS', 'BACS', 'CHAPS'])),
                ['sourceAccountId', 'beneficiaryName', 'sortCode', 'accountNumber', 'amount', 'reference', 'scheme'], 'Domestic payment instruction.'),
       status_docs='ACCEPTED (submitted to the scheme) or REJECTED (not sent; see rejectionReason).'),
    op('get_payment_status', 'GET', '/payments/v3/domestic-payments/status',
       'Get the latest status of a domestic payment by payment ID (format PAY-NNNNNNNNNN), including settlement time or rejection reason.',
       tlookup('paymentId', {p['paymentId']: p for p in d.PAYMENTS}, 'payment',
               ('^PAY-[0-9]{10}$', dict(paymentId=qv('paymentId'), sourceAccountId=None, beneficiaryName=None, amount=None, currency='GBP', scheme=None, status='COMPLETED',
                                        createdAt=None, settledAt=NOW, rejectionReason=None, reference=None))),
       params=[q('paymentId', S('Payment identifier returned by initiate_domestic_payment, format PAY-NNNNNNNNNN.', pattern=r'^PAY-[0-9]{10}$'))],
       status_docs='PENDING, COMPLETED or REJECTED.'),
    op('list_standing_orders', 'GET', '/payments/v3/standing-orders',
       'List standing orders on an account with amount, frequency, next payment date and the outcome of the last execution.',
       tlookup('accountId', {a['accountId']: dict(accountId=a['accountId'], standingOrders=[{k: v for k, v in s.items() if k != 'accountId'} for s in d.STANDING_ORDERS if s['accountId'] == a['accountId']])
                             for a in d.ACCOUNTS if any(s['accountId'] == a['accountId'] for s in d.STANDING_ORDERS)}, 'account',
               ('^ACC-[0-9]{8}$', dict(accountId=qv('accountId'), standingOrders=[]))),
       params=[q('accountId', ID['account'])]),
    op('cancel_standing_order', 'DELETE', '/payments/v3/standing-orders',
       'Cancel a standing order with immediate effect. Payments already submitted today are not recalled. Requires an explicit customer instruction.',
       [trule('cancelled', dict(standingOrderId=qv('standingOrderId'), status='CANCELLED', effectiveFrom=days(0), message='Standing order cancelled; no further payments will be made.'),
              query={'standingOrderId': rx('^SO-[0-9]{6}$')}), not_found('standing order', 'standingOrderId')],
       params=[q('standingOrderId', S('Standing order identifier from list_standing_orders, format SO-NNNNNN.', pattern=r'^SO-[0-9]{6}$'))]),
])

# ================================================= 4. international payments
_fx_quote_rules = []
for _sell, _buy in d.FX_QUOTE_PAIRS:
    _mid = d.fx_mid(_sell, _buy)
    _rate = round(_mid * (1 - d.CUSTOMER_FX_MARGIN), 6 if _mid < 10 else 4)
    _fx_quote_rules.append(trule((_sell + '-' + _buy).lower(), dict(quoteId='FXQ-' + rnd(8), sellCurrency=_sell, buyCurrency=_buy, sellAmount=Raw(qv('sellAmount')),
                                                                   customerRate=_rate, midMarketRate=_mid, marginPercent=d.CUSTOMER_FX_MARGIN * 100,
                                                                   estimatedBuyAmount=Raw("{{math request.query.sellAmount '*' " + str(_rate) + '}}'),
                                                                   fee=15.0 if _sell == 'GBP' else 20.0, expiresAt="{{now offset='30 seconds'}}"),
                                  query={'sellCurrency': eq(_sell), 'buyCurrency': eq(_buy)}))
_fx_quote_rules.append(rule('unsupported-pair', ok({'error': 'UNSUPPORTED_CURRENCY_PAIR', 'message': 'Pair not offered for payments'}, 422), priority=900))

_currencies = S('ISO 4217 currency code.', enum=d.FX_CURRENCIES)
_intl_ok = dict(paymentId='IPY-2026' + rnd(6), uetr=UUID, status='SUBMITTED', sourceAccountId=jb('sourceAccountId'), beneficiaryName=jb('beneficiaryName'),
                amount=Raw(jb('amount')), currency=jb('currency'), statusDescription='Sent via SWIFT gpi; typically credited within 24 hours.', trackingInfo='Use track_swift_payment with the uetr.', createdAt=NOW)

upstream('international-payments', 'International Payments (SWIFT gpi)', 'cross-border-payments',
         'Cross-border payments over SWIFT gpi: FX quotes for payments, payment initiation with compliance checks and end-to-end tracking by UETR.', [
    op('get_fx_quote', 'GET', '/swift/v2/fx-quotes',
       'Get a firm FX quote for an international payment, valid for 30 seconds. Shows customer rate (including the bank margin), mid-market rate, fee and estimated amount received. '
       'Supported pairs: GBP to USD/EUR/AED/CHF/JPY, USD to GBP/EUR/AED, EUR to GBP/USD; other pairs return HTTP 422.',
       _fx_quote_rules, params=[q('sellCurrency', S('Currency debited from the customer account.', enum=['GBP', 'USD', 'EUR'])),
                                q('buyCurrency', S('Currency the beneficiary receives.', enum=['GBP', 'USD', 'EUR', 'AED', 'CHF', 'JPY'])),
                                q('sellAmount', N('Amount to convert, in the sell currency.', exclusiveMinimum=0, maximum=10000000))]),
    op('initiate_international_payment', 'POST', '/swift/v2/payments',
       'Send an international payment via SWIFT gpi. Moves money and cannot be recalled once released. Payments are screened against sanctions lists; potential matches are HELD '
       'for compliance review and accounts with KYC restrictions are REJECTED. Obtain a quote with get_fx_quote and explicit customer confirmation first. '
       'Returns the UETR for track_swift_payment.',
       [trule('sanctions-hold', dict(_intl_ok, status='HELD_FOR_COMPLIANCE_REVIEW', statusDescription='Payment held for compliance review. Do not disclose the reason to the customer; advise that checks are in progress.'),
              status=201, priority=5, match=[body_rx('beneficiaryName', '(?i).*volkov.*')]),
        trule('kyc-restricted', dict(_intl_ok, uetr=None, status='REJECTED', statusDescription='Account restricted: KYC refresh required before international payments can be made.'),
              status=201, priority=6, match=[body_eq('sourceAccountId', 'ACC-10010601')]),
        trule('submitted', _intl_ok, status=201, priority=50)],
       body=obj(dict(sourceAccountId=ID['account'], beneficiaryName=S('Beneficiary name as held by the beneficiary bank.', pattern=NAME_TEXT),
                     beneficiaryIban=S('Beneficiary IBAN or account number.', pattern=r'^[A-Z0-9]{8,34}$'),
                     beneficiaryBic=S('Beneficiary bank BIC (8 or 11 characters).', pattern=r'^[A-Z]{6}[A-Z0-9]{2}([A-Z0-9]{3})?$'),
                     beneficiaryCountry=S('Beneficiary country, ISO 3166 alpha-2, e.g. US.', pattern=r'^[A-Z]{2}$'),
                     amount=N('Amount in the payment currency.', exclusiveMinimum=0, maximum=10000000, multipleOf=0.01), currency=_currencies,
                     purposeCode=S('Payment purpose.', enum=['GOODS', 'SERVICES', 'SALARY', 'FAMILY_SUPPORT', 'PROPERTY', 'EDUCATION', 'INVESTMENT', 'OTHER']),
                     chargeBearer=S('Who pays fees: SHA shared (default), OUR sender pays all, BEN beneficiary pays all.', enum=['SHA', 'OUR', 'BEN']),
                     quoteId=S('FX quote identifier from get_fx_quote when converting currency.', pattern=r'^FXQ-[0-9]{8}$')),
                ['sourceAccountId', 'beneficiaryName', 'beneficiaryIban', 'beneficiaryBic', 'beneficiaryCountry', 'amount', 'currency', 'purposeCode', 'chargeBearer'],
                'International payment instruction.'),
       status_docs='SUBMITTED, HELD_FOR_COMPLIANCE_REVIEW or REJECTED.'),
    op('track_swift_payment', 'GET', '/swift/v2/payments/tracker',
       'Track a cross-border payment end to end with SWIFT gpi by UETR: current status, banks the payment passed through and fees deducted. '
       'Status codes: ACSP in progress, ACCC credited to beneficiary, ON_HOLD held at a bank, RJCT rejected.',
       tlookup('uetr', {p['uetr']: dict({k: v for k, v in p.items() if k != 'hops'}, hops=[dict(bic=h[0], bank=h[1], timestamp=h[2], status=h[3]) for h in p['hops']])
                        for p in d.SWIFT_PAYMENTS}, 'payment',
               ('^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$',
                dict(uetr=qv('uetr'), paymentId=None, customerId=None, debtorAccountId=None, beneficiaryName=None, beneficiaryBic=None, amount=None, currency=None,
                     status='ACSP', statusDescription='In progress at the next bank in the chain', initiatedAt=None, completedAt=None, totalFees=0.0,
                     hops=[dict(bic=d.BANK_BIC, bank='Meridian Bank London', timestamp=NOW, status='ACSP')]))),
       params=[q('uetr', S('SWIFT gpi UETR (lowercase UUID), e.g. e3b0c442-98fc-4c1c-9a3f-6d2b1a7c5e01.', pattern=r'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'))],
       status_docs='gpi status: ACSP (in progress), ACCC (credited), ON_HOLD or RJCT (rejected).'),
])

# ======================================================= 5. card management
_card_ref = jb('cardId')
upstream('card-management', 'Card Management', 'cards-issuing',
         'Debit and credit card lifecycle: card inventory, details, temporary freeze, permanent block with replacement, unfreeze and spending controls.', [
    op('list_customer_cards', 'GET', '/cards/v1/cards',
       'List all debit and credit cards of a customer with masked number, status, linked account, limits and whether a fraud alert is open.',
       lookup('customerId', per_customer(lambda cid: dict(customerId=cid, cards=[dict(cardId=c['cardId'], cardType=c['cardType'], productName=c['productName'], maskedPan=c['maskedPan'],
                                                                                    status=c['status'], expiryDate=c['expiryDate'], linkedAccountId=c['linkedAccountId'], openFraudAlert=c['openFraudAlert'])
                                                                               for c in d.by_customer(d.CARDS, cid)])), 'customer'),
       params=[q('customerId', ID['customer'])], status_docs='ACTIVE, FROZEN (temporary, reversible), BLOCKED (permanent) or EXPIRED.'),
    op('get_card_details', 'GET', '/cards/v1/cards/details',
       'Get details of one card: product, masked number, status and reason, expiry, credit limit and balance (credit cards), daily limits and online/contactless controls.',
       lookup('cardId', {c['cardId']: card_public(c) for c in d.CARDS}, 'card'), params=[q('cardId', ID['card'])],
       status_docs='ACTIVE, FROZEN (temporary, reversible), BLOCKED (permanent) or EXPIRED.'),
    op('freeze_card', 'POST', '/cards/v1/cards/freeze',
       'Stop a card from being used. Reason CUSTOMER_REQUEST or SUSPECTED_FRAUD applies a temporary, reversible freeze. Reason LOST or STOLEN permanently blocks the card and '
       'orders a replacement to the registered address (cannot be undone). Confirm with the customer before calling.',
       [trule('permanent-block', dict(cardId=_card_ref, status='BLOCKED', reason=jb('reason'), updatedAt=NOW, message='Card permanently blocked. A replacement card has been ordered to the registered address.',
                                      replacementCard=dict(cardId='CRD-' + rnd(4), deliveryEstimate=days(4))), status=200, priority=5, match=[body_rx('reason', '^(LOST|STOLEN)$')]),
        trule('temporary-freeze', dict(cardId=_card_ref, status='FROZEN', reason=jb('reason'), updatedAt=NOW, message='Card temporarily frozen. It can be unfrozen with unfreeze_card.',
                                       replacementCard=None), priority=20)],
       body=obj(dict(cardId=ID['card'], reason=S('Why the card is being stopped. LOST and STOLEN are permanent.', enum=['CUSTOMER_REQUEST', 'SUSPECTED_FRAUD', 'LOST', 'STOLEN'])),
                ['cardId', 'reason'], 'Card freeze or block request.'),
       status_docs='FROZEN (temporary) or BLOCKED (permanent).'),
    op('unfreeze_card', 'POST', '/cards/v1/cards/unfreeze',
       'Reactivate a temporarily frozen card. Not possible for permanently blocked cards or while a fraud alert is open on the card (returns status REJECTED with the reason).',
       [trule('fraud-alert-open', dict(cardId=_card_ref, status='REJECTED', reason='OPEN_FRAUD_ALERT', updatedAt=NOW,
                                       message='The card has an unresolved fraud alert (FRA-400101). The fraud team must resolve it before the card can be unfrozen.'),
              priority=5, match=[body_eq('cardId', 'CRD-5008')]),
        trule('unfrozen', dict(cardId=_card_ref, status='ACTIVE', reason='CUSTOMER_REQUEST', updatedAt=NOW, message='Card reactivated and ready to use.'), priority=20)],
       body=obj(dict(cardId=ID['card']), ['cardId'], 'Card unfreeze request.'), status_docs='ACTIVE when reactivated, REJECTED when not allowed.'),
    op('update_card_spending_limits', 'PATCH', '/cards/v1/cards/limits',
       'Change card controls: daily purchase limit, daily ATM limit and whether online (card-not-present) payments are allowed. Supply only the controls that change. '
       'Limits cannot exceed the product maximum (purchases 10,000 GBP, ATM 1,000 GBP per day).',
       [trule('updated', dict(cardId=_card_ref, status='UPDATED', updatedAt=NOW, message='Card controls updated with immediate effect.'))],
       body=obj(dict(cardId=ID['card'], dailyPurchaseLimit=N('New daily purchase limit in GBP.', minimum=0, maximum=10000),
                     dailyAtmLimit=N('New daily ATM withdrawal limit in GBP.', minimum=0, maximum=1000),
                     onlinePaymentsEnabled=B('Allow (true) or block (false) online and phone payments.')), ['cardId'],
                'Card control changes. Include at least one control.'), status_docs='UPDATED when applied.'),
])

# ====================================================== 6. card disputes
upstream('card-disputes', 'Card Disputes & Chargebacks', 'disputes-chargebacks',
         'Card transaction history with merchant details, dispute intake, provisional credit and chargeback case tracking.', [
    op('list_card_transactions', 'GET', '/disputes/v1/card-transactions',
       'List recent transactions on a card with merchant name, merchant category code, country, card-present flag, authorisation status and any existing dispute reference. '
       'Use to identify the exact transaction before opening a dispute or assessing fraud.',
       lookup('cardId', {c['cardId']: dict(cardId=c['cardId'], transactions=d.card_transactions(c['cardId'])) for c in d.CARDS}, 'card'),
       params=[q('cardId', ID['card']), q('limit', I('Maximum number of transactions (1-100).', minimum=1, maximum=100), required=False, default=20)],
       status_docs='AUTHORISED (pending), SETTLED or DECLINED.'),
    op('open_card_dispute', 'POST', '/disputes/v1/disputes',
       'Open a dispute for a card transaction the customer recognises but has a problem with (goods not received, service cancelled, duplicate charge, incorrect amount, cancelled subscription). '
       'For unrecognised transactions use the fraud process instead. Eligible disputes receive provisional credit within one working day.',
       [trule('opened', dict(disputeId='DSP-6' + rnd(5), cardId=_card_ref, transactionId=jb('transactionId'), reasonCode=jb('reasonCode'), status='OPEN', openedOn=days(0),
                             provisionalCreditIssued=False, schemeDeadline=days(45), nextStep='Evidence review by the disputes team within 2 working days; provisional credit decision within 1 working day.'),
              status=201)],
       body=obj(dict(cardId=ID['card'], transactionId=S('Card transaction ID from list_card_transactions, e.g. CTX-80080001.', pattern=r'^CTX-[0-9]{8}$|^TXN-[0-9]{6}-[0-9]{2}$'),
                     reasonCode=S('Dispute reason.', enum=['GOODS_NOT_RECEIVED', 'SERVICE_NOT_PROVIDED', 'DUPLICATE_CHARGE', 'INCORRECT_AMOUNT', 'CANCELLED_RECURRING', 'NOT_AS_DESCRIBED']),
                     description=S('What happened, in the customer\'s words (10-500 characters).', pattern=FREE_TEXT)),
                ['cardId', 'transactionId', 'reasonCode', 'description'], 'Dispute details.'),
       status_docs='OPEN when created.'),
    op('get_dispute_case', 'GET', '/disputes/v1/disputes/case',
       'Get a dispute case: transaction, reason, status, provisional credit, card scheme deadline, assigned analyst and timeline of events.',
       tlookup('disputeId', {x['disputeId']: with_staff(x, 'assignedAnalyst') for x in d.DISPUTES}, 'dispute',
               ('^DSP-[0-9]{6}$', dict(disputeId=qv('disputeId'), customerId=None, cardId=None, transactionId=None, merchantName=None, amount=None, currency='GBP', reasonCode=None,
                                       status='UNDER_REVIEW', openedOn=None, provisionalCreditIssued=False, provisionalCreditAmount=0.0, schemeDeadline=None, assignedAnalyst=None,
                                       timeline=['Dispute received and queued for evidence review']))),
       params=[q('disputeId', S('Dispute identifier, format DSP-NNNNNN.', pattern=r'^DSP-[0-9]{6}$'))],
       status_docs='OPEN, UNDER_REVIEW, CHARGEBACK_SUBMITTED, RESOLVED_IN_CUSTOMER_FAVOUR or RESOLVED_IN_MERCHANT_FAVOUR.'),
])

# ===================================================== 7. fraud detection
_generic_score = dict(transactionId=qv('transactionId'), score=0.06, decision='APPROVE', reasons=['Consistent with customer spending pattern'], modelVersion='fraud-gbm-2026.09')
upstream('fraud-detection', 'Fraud Detection Engine', 'fraud-strategy',
         'Real-time fraud scoring and alert management for card and payment transactions.', [
    op('list_fraud_alerts', 'GET', '/fraud/v2/alerts',
       'List fraud alerts for a customer (open and closed), with risk score, triggering rule, linked card and transaction, and recommended action.',
       tlookup('customerId', {cid: dict(customerId=cid, alerts=[with_staff({k: v for k, v in a.items() if k != 'customerId'}, 'assignedAnalyst') for a in d.FRAUD_ALERTS if a['customerId'] == cid])
                              for cid in {a['customerId'] for a in d.FRAUD_ALERTS}}, 'customer',
               ('^CUST-[0-9]{6}$', dict(customerId=qv('customerId'), alerts=[]))),
       params=[q('customerId', ID['customer'])], status_docs='OPEN, CLOSED_GENUINE or CLOSED_CONFIRMED_FRAUD.'),
    op('get_transaction_risk_score', 'GET', '/fraud/v2/risk-scores',
       'Get the fraud model score (0 genuine to 1 fraudulent), recommended decision and main reasons for a card (CTX-...) or account (TXN-...) transaction.',
       [rule(k.lower(), ok(dict(transactionId=k, **v, modelVersion='fraud-gbm-2026.09')), query={'transactionId': eq(k)}) for k, v in d.RISK_SCORES.items()] +
       [trule('baseline', _generic_score, priority=50)],
       params=[q('transactionId', S('Card transaction ID (CTX-NNNNNNNN) or account transaction ID (TXN-NNNNNN-NN).', pattern=r'^CTX-[0-9]{8}$|^TXN-[0-9]{6}-[0-9]{2}$'))],
       docs={'decision': 'APPROVE, REVIEW or DECLINE.'}),
    op('resolve_fraud_alert', 'POST', '/fraud/v2/alerts/resolution',
       'Record the outcome of a fraud alert after speaking to the customer. CONFIRMED_FRAUD triggers card reissue and chargeback; GENUINE releases blocks and whitelists the merchant for 30 days. '
       'Only call once the customer has explicitly confirmed or denied the transaction.',
       [trule('confirmed', dict(alertId=jb('alertId'), outcome='CONFIRMED_FRAUD', resolvedAt=NOW,
                                nextActions=['Card permanently blocked and replacement ordered', 'Fraud chargeback raised for the disputed transaction', 'Customer refund within 1 working day']),
              priority=5, match=[body_eq('outcome', 'CONFIRMED_FRAUD')]),
        trule('genuine', dict(alertId=jb('alertId'), outcome='GENUINE', resolvedAt=NOW,
                              nextActions=['Alert closed', 'Merchant trusted for 30 days', 'Unfreeze the card if it was frozen']), priority=20)],
       body=obj(dict(alertId=S('Fraud alert identifier, format FRA-NNNNNN.', pattern=r'^FRA-[0-9]{6}$'),
                     outcome=S('Customer-confirmed outcome.', enum=['CONFIRMED_FRAUD', 'GENUINE']),
                     customerVerified=B('True if the customer passed identity verification on this contact.'),
                     notes=S('Investigator notes (10-500 characters).', pattern=FREE_TEXT)), ['alertId', 'outcome', 'customerVerified'], 'Alert resolution.')),
])

# ======================================================== 8. AML screening
_screen = dict(screeningId='SCR-' + rnd(8), screenedName=jb('fullName'), screenedAt=NOW)
upstream('aml-screening', 'AML & Sanctions Screening', 'financial-crime-compliance',
         'Sanctions and PEP screening, AML transaction monitoring alerts and investigation case management. Information is confidential: never disclose to customers (tipping-off).', [
    op('screen_party', 'POST', '/fcc/v1/screening',
       'Screen a person or organisation against sanctions lists, PEP lists and adverse media. Returns CLEAR, POTENTIAL_MATCH (escalate to Financial Crime Compliance; do not proceed) or PEP_MATCH (enhanced due diligence required).',
       [trule('sanctions-potential-match', dict(_screen, matchStatus='POTENTIAL_MATCH', hits=[dict(list='UK HMT consolidated sanctions list (simulated)', entityName='VOLKOV TRADING FZE', matchScore=0.87,
                                                                                                listingReason='Asset freeze - circumvention of trade restrictions')],
                                                recommendedAction='Escalate to Financial Crime Compliance. Do not process related payments.'), priority=5, match=[body_rx('fullName', '(?i).*volkov.*')]),
        trule('pep-match', dict(_screen, matchStatus='PEP_MATCH', hits=[dict(list='Politically exposed persons (simulated)', entityName='SERGEI IVANENKO', matchScore=0.93,
                                                                             listingReason='Former deputy minister of transport')],
                                recommendedAction='Apply enhanced due diligence and obtain senior management approval.'), priority=6, match=[body_rx('fullName', '(?i).*ivanenko.*')]),
        trule('clear', dict(_screen, matchStatus='CLEAR', hits=[], recommendedAction='No action required.'), priority=50)],
       body=obj(dict(fullName=S('Full name of the person or organisation to screen.', pattern=NAME_TEXT),
                     partyType=S('Type of party.', enum=['INDIVIDUAL', 'ORGANISATION']),
                     country=S('Country of residence or registration, ISO 3166 alpha-2.', pattern=r'^[A-Z]{2}$'),
                     dateOfBirth=S('Date of birth for individuals (YYYY-MM-DD), improves match accuracy.', pattern=DATE)),
                ['fullName', 'partyType', 'country'], 'Party to screen.')),
    op('list_aml_alerts', 'GET', '/fcc/v1/aml/alerts',
       'List AML monitoring cases. Filter by customer, or omit customerId to list all cases across the bank (newest first) for oversight reporting.',
       [rule(cid.lower(), ok(dict(customerId=cid, alerts=[with_staff(dict(caseId=c['caseId'], customerId=c['customerId'], alertType=c['alertType'], status=c['status'], priority=c['priority'],
                                                                         openedOn=c['openedOn'], dueDate=c['dueDate'], assignedInvestigator=c['assignedInvestigator']), 'assignedInvestigator')
                                                          for c in d.AML_CASES if c['customerId'] == cid])), query={'customerId': eq(cid)})
        for cid in sorted({c['customerId'] for c in d.AML_CASES})] +
       [trule('no-alerts', dict(customerId=qv('customerId'), alerts=[]), priority=40, query={'customerId': rx('^CUST-[0-9]{6}$')}),
        rule('all', ok(dict(customerId=None, alerts=[with_staff(dict(caseId=c['caseId'], customerId=c['customerId'], alertType=c['alertType'], status=c['status'], priority=c['priority'],
                                                                     openedOn=c['openedOn'], dueDate=c['dueDate'], assignedInvestigator=c['assignedInvestigator']), 'assignedInvestigator') for c in d.AML_CASES])),
             priority=60)],
       params=[q('customerId', ID['customer'], required=False)], docs={'customerId': 'Customer filter applied; null when listing all cases.'},
       status_docs='OPEN, UNDER_INVESTIGATION, ESCALATED or CLOSED_NO_FURTHER_ACTION.'),
    op('get_aml_case', 'GET', '/fcc/v1/aml/cases',
       'Get an AML investigation case: scenario, summary, linked transactions and payments, screening hits, assigned investigator, SAR decision and next steps.',
       lookup('caseId', {c['caseId']: with_staff(c, 'assignedInvestigator') for c in d.AML_CASES}, 'AML case'),
       params=[q('caseId', S('AML case identifier, format AML-NNNNNN.', pattern=r'^AML-[0-9]{6}$'))],
       status_docs='OPEN, UNDER_INVESTIGATION, ESCALATED or CLOSED_NO_FURTHER_ACTION.'),
])

# ========================================================= 9. KYC onboarding
def kyc_status(cid):
    k = d.KYC[cid]
    return dict(customerId=cid, status=k['status'], riskRating=d.CUSTOMER[cid]['riskRating'], lastReviewDate=k['lastReviewDate'], nextReviewDue=k['nextReviewDue'],
                documents=[dict(type=t, status=s, expiryDate=e) for t, s, e in k['documents']], outstandingRequirements=k['outstanding'], openCaseId=k['openCaseId'])


_doc_rules = [rule('limited-company-high-risk', ok(dict(customerType='LIMITED_COMPANY', riskRating='HIGH', requiredDocuments=d.KYC_DOCUMENTS['LIMITED_COMPANY'], enhancedDueDiligence=d.KYC_EDD_DOCUMENTS)),
                   priority=5, query={'customerType': eq('LIMITED_COMPANY'), 'riskRating': eq('HIGH')})]
_doc_rules += [trule(t.lower().replace('_', '-'), dict(customerType=t, riskRating=qv('riskRating'), requiredDocuments=docs, enhancedDueDiligence=[]), query={'customerType': eq(t)})
               for t, docs in d.KYC_DOCUMENTS.items()]

upstream('kyc-onboarding', 'KYC & Onboarding', 'kyc-operations',
         'Customer due diligence: KYC status, document requirements by customer type and risk, and periodic or trigger-event review cases.', [
    op('get_kyc_status', 'GET', '/kyc/v1/status',
       'Get a customer\'s KYC status, risk rating, last and next review dates, documents on file with expiry, outstanding requirements and any open KYC case.',
       lookup('customerId', per_customer(kyc_status), 'customer'), params=[q('customerId', ID['customer'])],
       status_docs='VERIFIED, REFRESH_REQUIRED, ENHANCED_DUE_DILIGENCE or REJECTED (customer level); document status VERIFIED, EXPIRED, REQUESTED or PENDING_VERIFICATION.'),
    op('list_required_kyc_documents', 'GET', '/kyc/v1/document-requirements',
       'List the identity and verification documents required for a customer type. High-risk limited companies also require enhanced due diligence documents.',
       _doc_rules, params=[q('customerType', S('Customer legal form.', enum=list(d.KYC_DOCUMENTS))),
                           q('riskRating', S('Customer risk rating; HIGH adds enhanced due diligence.', enum=['LOW', 'MEDIUM', 'HIGH']), required=False, default='LOW')]),
    op('start_kyc_refresh', 'POST', '/kyc/v1/refresh-cases',
       'Open a KYC refresh case for a customer (periodic review, trigger event, expired document or customer request). Returns the case ID, required documents and due date. '
       'If a case is already open, returns ALREADY_IN_PROGRESS with the existing case ID.',
       [trule('already-open-' + cid.lower(), dict(caseId=d.KYC[cid]['openCaseId'], customerId=cid, status='ALREADY_IN_PROGRESS', existingCaseId=d.KYC[cid]['openCaseId'],
                                                  requiredDocuments=d.KYC[cid]['outstanding'], dueDate=d.KYC[cid]['nextReviewDue']), priority=5, match=[body_eq('customerId', cid)])
        for cid in sorted(c for c, k in d.KYC.items() if k['openCaseId'])] +
       [trule('opened', dict(caseId='KYC-55' + rnd(4), customerId=jb('customerId'), status='OPEN', existingCaseId=None,
                             requiredDocuments=['Proof of address dated within the last 3 months', 'Confirmation of employment or source of income'], dueDate=days(30)),
              status=201, priority=50)],
       body=obj(dict(customerId=ID['customer'], reason=S('Why the review is needed.', enum=['PERIODIC_REVIEW', 'TRIGGER_EVENT', 'DOCUMENT_EXPIRED', 'CUSTOMER_REQUEST']),
                     preferredChannel=S('Channel for the document request; must be one the customer consented to.', enum=['EMAIL', 'SECURE_MESSAGE', 'POST', 'BRANCH'])),
                ['customerId', 'reason', 'preferredChannel'], 'KYC refresh request.'),
       status_docs='OPEN or ALREADY_IN_PROGRESS.'),
])

# =================================================== 10. credit bureau gateway
def bureau(cid):
    b = d.CREDIT_BUREAU[cid]
    business = d.CUSTOMER[cid]['customerType'] != 'INDIVIDUAL'
    return dict(customerId=cid, bureau='Sterling Credit Reference Agency (simulated)', scoreScale='0-100 commercial' if business else '0-999 consumer',
                reportDate='2026-10-28', **b)


upstream('credit-bureau-gateway', 'Credit Bureau Gateway', 'credit-risk-analytics',
         'Credit reference agency reports and internal credit risk grading with affordability for consumer and business customers.', [
    op('get_credit_bureau_report', 'GET', '/credit/v1/bureau-reports',
       'Get the external credit bureau report: score and band, open credit accounts, missed payments, defaults, county court judgments, recent hard searches and utilisation. '
       'A soft search; it does not affect the customer\'s credit file. Consumer scores are 0-999; business scores are 0-100.',
       lookup('customerId', per_customer(bureau), 'customer'), params=[q('customerId', ID['customer'])]),
    op('get_internal_risk_grade', 'GET', '/credit/v1/risk-grades',
       'Get the bank\'s internal risk grade (1 best to 10 worst), 12-month probability of default, estimated monthly disposable income, maximum recommended unsecured exposure and credit policy flags.',
       lookup('customerId', per_customer(lambda cid: dict(customerId=cid, **d.RISK_GRADES[cid], gradedAt=d.AS_OF)), 'customer'), params=[q('customerId', ID['customer'])]),
])

# ===================================================== 11. loan origination
def payment_per_1000(rate):
    return [dict(termMonths=t, monthlyPayment=round(1000 * (rate / 1200) / (1 - (1 + rate / 1200) ** -t), 2)) for t in (12, 24, 36, 48, 60, 84, 120, 300)]


_quote_products = {
    'PL-STANDARD': dict(representativeApr=9.9, rateType='FIXED', minAmount=1000.0, maxAmount=35000.0, minTermMonths=12, maxTermMonths=84, fees=['No arrangement fee', 'Early settlement charge up to 58 days interest']),
    'MTG-FIX5': dict(representativeApr=6.4, rateType='FIXED', minAmount=25000.0, maxAmount=2000000.0, minTermMonths=60, maxTermMonths=420, fees=['Product fee 999 GBP', 'Early repayment charge 5%-1% during fixed period']),
    'MTG-TRACKER': dict(representativeApr=5.1, rateType='TRACKER', minAmount=25000.0, maxAmount=2000000.0, minTermMonths=60, maxTermMonths=420, fees=['No product fee', 'No early repayment charges']),
    'BL-TERM': dict(representativeApr=8.2, rateType='VARIABLE', minAmount=25000.0, maxAmount=1000000.0, minTermMonths=12, maxTermMonths=120, fees=['Arrangement fee 1% of the loan amount']),
}
_rates = {'PL-STANDARD': 9.9, 'MTG-FIX5': 4.29, 'MTG-TRACKER': 4.79, 'BL-TERM': 7.45}


def application_public(a):
    out = with_staff(a, 'underwriter')
    return out


_app_generic = dict(applicationId=qv('applicationId'), customerId=None, productCode=None, requestedAmount=None, termMonths=None, purpose=None, status='RECEIVED',
                    decision='PENDING', submittedAt=None, underwriter=None, outstandingRequirements=['Automated checks in progress'], offer=None, declineReasons=[])
_app_submit = dict(applicationId='APP-88' + rnd(4), customerId=jb('customerId'), productCode=jb('productCode'), requestedAmount=Raw(jb('amount')),
                   termMonths=Raw(jb('termMonths')), submittedAt=NOW)

upstream('loan-origination', 'Loan Origination', 'lending-origination',
         'Lending quotes, applications and decisions for personal loans, mortgages and business term loans.', [
    op('get_loan_quote', 'GET', '/lending/v1/quotes',
       'Get an indicative quote for a lending product: representative APR, rate type, amount and term limits, fees and the monthly payment per 1,000 borrowed for common terms. '
       'Multiply monthlyPaymentPer1000 for the chosen term by amount/1000 to estimate the monthly payment. Indicative only; the final rate depends on the credit decision.',
       [trule(p.lower(), dict(productCode=p, requestedAmount=Raw(qv('amount')), termMonths=Raw(qv('termMonths')), **v, monthlyPaymentPer1000=payment_per_1000(_rates[p]),
                              disclaimer='Indicative quote. Representative example: borrowing 10,000 GBP over 60 months. Subject to status and credit assessment.'),
              query={'productCode': eq(p)}) for p, v in _quote_products.items()] + [not_found('quotable product', 'productCode')],
       params=[q('productCode', S('Lending product to quote.', enum=list(_quote_products))), q('amount', N('Amount to borrow in GBP.', minimum=1000, maximum=2000000)),
               q('termMonths', I('Repayment term in months.', minimum=12, maximum=420))]),
    op('submit_loan_application', 'POST', '/lending/v1/applications',
       'Submit a loan application for an existing customer. Runs automated credit and affordability checks and returns an immediate decision: APPROVED_IN_PRINCIPLE (with offer), REFERRED (manual underwriting) or DECLINED (with reasons). '
       'Performs a hard credit search. Only submit after the customer confirms product, amount, term and purpose.',
       [trule('declined-arrears', dict(_app_submit, purpose=jb('purpose'), status='DECLINED', decision='DECLINE', offer=None,
                                       declineReasons=['Missed payments on existing Meridian Bank personal loan LN-700102', 'Affordability below policy threshold'],
                                       outstandingRequirements=[]), status=201, priority=5, match=[body_eq('customerId', 'CUST-100102')]),
        trule('referred-aml', dict(_app_submit, purpose=jb('purpose'), status='REFERRED', decision='REFER', offer=None, declineReasons=[],
                                   outstandingRequirements=['Referred to Financial Crime Compliance before any new lending decision']), status=201, priority=6,
              match=[body_eq('customerId', 'CUST-200201')]),
        trule('approved', dict(_app_submit, purpose=jb('purpose'), status='APPROVED_IN_PRINCIPLE', decision='APPROVE', declineReasons=[],
                               offer=dict(amount=Raw(jb('amount')), termMonths=Raw(jb('termMonths')), interestRate=9.9, monthlyPayment=None, arrangementFee=0.0, offerExpiresOn=days(30)),
                               outstandingRequirements=['Accept the offer in online banking', 'Upload latest payslip if requested']), status=201, priority=50)],
       body=obj(dict(customerId=ID['customer'], productCode=S('Lending product.', enum=list(_quote_products)), amount=N('Requested amount in GBP.', minimum=1000, maximum=2000000),
                     termMonths=I('Requested term in months.', minimum=12, maximum=420),
                     purpose=S('Loan purpose.', enum=['HOME_IMPROVEMENT', 'CAR_PURCHASE', 'DEBT_CONSOLIDATION', 'HOUSE_PURCHASE', 'REMORTGAGE', 'WORKING_CAPITAL', 'EQUIPMENT', 'OTHER']),
                     annualIncome=N('Gross annual income (individuals) or turnover (businesses) in GBP.', minimum=0),
                     employmentStatus=S('Employment status for individuals; TRADING for businesses.', enum=['EMPLOYED', 'SELF_EMPLOYED', 'RETIRED', 'STUDENT', 'UNEMPLOYED', 'TRADING'])),
                ['customerId', 'productCode', 'amount', 'termMonths', 'purpose', 'annualIncome', 'employmentStatus'], 'Loan application.'),
       status_docs='APPROVED_IN_PRINCIPLE, REFERRED or DECLINED.', docs={'monthlyPayment': 'Contractual monthly payment; null until the offer is finalised.'}),
    op('get_loan_application', 'GET', '/lending/v1/applications/status',
       'Get a loan application: requested amount and term, status, decision, assigned underwriter, outstanding requirements, offer terms or decline reasons.',
       tlookup('applicationId', {a['applicationId']: application_public(a) for a in d.LOAN_APPLICATIONS}, 'application', ('^APP-[0-9]{6}$', _app_generic)),
       params=[q('applicationId', S('Application identifier, format APP-NNNNNN.', pattern=r'^APP-[0-9]{6}$'))],
       status_docs='RECEIVED, UNDER_REVIEW, APPROVED, APPROVED_IN_PRINCIPLE, REFERRED or DECLINED.'),
])

# ====================================================== 12. loan servicing
upstream('loan-servicing', 'Loan Servicing', 'lending-servicing',
         'Administration of live mortgages, personal loans, business loans and revolving credit facilities: balances, schedules and settlement figures.', [
    op('list_customer_loans', 'GET', '/loans/v2/loans',
       'List a customer\'s live loans and facilities with outstanding balance, rate, monthly payment, next payment date and arrears status.',
       lookup('customerId', per_customer(lambda cid: dict(customerId=cid, loans=[dict(loanId=l['loanId'], loanType=l['loanType'], productName=l['productName'], outstandingPrincipal=l['outstandingPrincipal'],
                                                                                    currency=l['currency'], interestRate=l['interestRate'], monthlyPayment=l['monthlyPayment'],
                                                                                    nextPaymentDate=l['nextPaymentDate'], status=l['status'], daysPastDue=l['daysPastDue'])
                                                                               for l in d.by_customer(d.LOANS, cid)])), 'customer'),
       params=[q('customerId', ID['customer'])], status_docs='PERFORMING, DELINQUENT, DEFAULTED or SETTLED.'),
    op('get_loan_details', 'GET', '/loans/v2/loans/details',
       'Get full loan details: original amount, outstanding principal, rate and fixed period, term, maturity, payments, arrears, collateral, loan-to-value and covenants.',
       lookup('loanId', {l['loanId']: dict(l, covenants=l.get('covenants', [])) for l in d.LOANS}, 'loan'), params=[q('loanId', ID['loan'])],
       status_docs='PERFORMING, DELINQUENT, DEFAULTED or SETTLED.'),
    op('get_repayment_schedule', 'GET', '/loans/v2/loans/schedule',
       'Get the upcoming repayment schedule for a loan with principal and interest split and projected balance after each installment. Facilities show interest-only payments.',
       lookup('loanId', {l['loanId']: dict(loanId=l['loanId'], currency=l['currency'], installments=d.repayment_schedule(l['loanId'])) for l in d.LOANS}, 'loan'),
       params=[q('loanId', ID['loan']), q('count', I('Number of upcoming installments to return (1-24).', minimum=1, maximum=24), required=False, default=6)],
       status_docs='NEXT for the next installment, DUE for later ones.'),
    op('get_early_settlement_quote', 'GET', '/loans/v2/loans/settlement-quote',
       'Get the amount required to repay a loan in full: outstanding principal, accrued interest, arrears and early repayment charge, with validity date and payment instructions.',
       lookup('loanId', {l['loanId']: d.early_settlement(l['loanId']) for l in d.LOANS}, 'loan'), params=[q('loanId', ID['loan'])]),
])

# ================================================== 13. collections & recovery
_arrears = [dict(caseId=c['caseId'], customerId=c['customerId'], fullName=d.CUSTOMER[c['customerId']]['fullName'], loanId=c['loanId'], daysPastDue=c['daysPastDue'],
                 arrearsAmount=c['arrearsAmount'], stage=c['stage'], vulnerabilityIndicators=c['vulnerabilityIndicators']) for c in d.COLLECTION_CASES]
upstream('collections-recovery', 'Collections & Recovery', 'collections-recovery',
         'Arrears management: delinquent accounts, collection cases with contact history and vulnerability flags, and affordable payment arrangements.', [
    op('list_arrears_accounts', 'GET', '/collections/v1/arrears',
       'List customers in arrears with days past due, overdue amount, collections stage and vulnerability indicators, most overdue first.',
       [rule('all', ok(dict(asOf=d.AS_OF, total=len(_arrears), accounts=_arrears)))],
       params=[q('minDaysPastDue', I('Only include accounts at least this many days past due.', minimum=1, maximum=365), required=False, default=1)],
       docs={'accounts': 'Accounts in arrears, most overdue first.'}),
    op('get_collection_case', 'GET', '/collections/v1/cases',
       'Get the collection case for a customer: stage, arrears, assigned collector, last contact, promise to pay, vulnerability indicators and recommended treatment. Returns HTTP 404 when the customer has no case.',
       lookup('customerId', {c['customerId']: with_staff(c, 'assignedCollector') for c in d.COLLECTION_CASES}, 'collection case'), params=[q('customerId', ID['customer'])],
       status_docs='OPEN, ARRANGEMENT_ACTIVE, BROKEN_ARRANGEMENT or CLOSED.'),
    op('propose_payment_arrangement', 'POST', '/collections/v1/arrangements',
       'Propose a payment arrangement on a collection case: a monthly amount on top of (or instead of) the contractual payment for a number of months. '
       'Arrangements up to 12 months within affordability are approved automatically; longer ones go to a team leader. Agree the amount with the customer first.',
       [trule('approved', dict(arrangementId='ARR-' + rnd(6), caseId=jb('caseId'), status='APPROVED', monthlyAmount=Raw(jb('monthlyAmount')), numberOfMonths=Raw(jb('numberOfMonths')),
                               firstPaymentDate=jb('firstPaymentDate'), message='Arrangement approved. Interest and charges are frozen while payments are kept.'), status=201, priority=20,
              match=[body_rx('numberOfMonths', '^([1-9]|1[0-2])$')]),
        trule('referred', dict(arrangementId='ARR-' + rnd(6), caseId=jb('caseId'), status='PENDING_TEAM_LEADER_APPROVAL', monthlyAmount=Raw(jb('monthlyAmount')),
                               numberOfMonths=Raw(jb('numberOfMonths')), firstPaymentDate=jb('firstPaymentDate'),
                               message='Arrangements longer than 12 months require team leader approval within 2 working days.'), status=201, priority=50)],
       body=obj(dict(caseId=S('Collection case identifier, format COL-NNNNNN.', pattern=r'^COL-[0-9]{6}$'), monthlyAmount=N('Agreed monthly payment in GBP.', exclusiveMinimum=0, maximum=100000),
                     numberOfMonths=I('Duration in months (1-36).', minimum=1, maximum=36), firstPaymentDate=S('First payment date (YYYY-MM-DD).', pattern=DATE)),
                ['caseId', 'monthlyAmount', 'numberOfMonths', 'firstPaymentDate'], 'Payment arrangement proposal.'),
       status_docs='APPROVED or PENDING_TEAM_LEADER_APPROVAL.'),
])

# ===================================================== 14. treasury markets
upstream('treasury-markets-data', 'Treasury Market Data & Liquidity', 'treasury-markets',
         'Treasury market data and balance sheet liquidity: FX reference rates, interest rate curves and the bank\'s liquidity position.', [
    op('get_fx_rates', 'GET', '/treasury/v1/fx-rates',
       'Get FX reference rates for a base currency against USD, EUR, GBP, CHF, JPY and AED: mid-market, bank buy and sell rates and 24-hour change. '
       'For a firm customer payment rate use get_fx_quote.',
       [rule(b.lower(), ok(dict(baseCurrency=b, asOf=d.AS_OF, rates=d.fx_table(b), source='Meridian Treasury composite (simulated)')), query={'baseCurrency': eq(b)}) for b in ('GBP', 'USD', 'EUR')],
       params=[q('baseCurrency', S('Base currency.', enum=['GBP', 'USD', 'EUR']), required=False, default='GBP')]),
    op('get_interest_rate_curve', 'GET', '/treasury/v1/rate-curves',
       'Get a benchmark interest rate curve (SONIA for GBP, EURIBOR for EUR, SOFR for USD) from overnight to 10 years.',
       [rule(c.lower(), ok(dict(curve=c, asOf=d.AS_OF, points=[dict(tenor=t, ratePercent=r) for t, r in pts], source='Meridian Treasury curve builder (simulated)')), query={'curve': eq(c)})
        for c, pts in d.RATE_CURVES.items()],
       params=[q('curve', S('Benchmark curve.', enum=list(d.RATE_CURVES)))]),
    op('get_liquidity_position', 'GET', '/treasury/v1/liquidity',
       'Get the bank\'s current liquidity position: Liquidity Coverage Ratio against regulatory and internal triggers, Net Stable Funding Ratio, HQLA, stressed outflows, positions by currency and active alerts.',
       [rule('current', ok(d.LIQUIDITY))], status_docs='WITHIN_APPETITE, EARLY_WARNING or BREACH.'),
])

# ======================================================== 15. trade finance
upstream('trade-finance', 'Trade Finance', 'trade-finance',
         'Documentary trade: import and export letters of credit, document discrepancies and bank guarantees.', [
    op('list_letters_of_credit', 'GET', '/trade/v1/letters-of-credit',
       'List a customer\'s import and export letters of credit with status, amount, expiry and counterparties.',
       tlookup('customerId', {cid: dict(customerId=cid, lettersOfCredit=[dict(lcReference=l['lcReference'], direction=l['direction'], status=l['status'], applicant=l['applicant'],
                                                                              beneficiary=l['beneficiary'], amount=l['amount'], currency=l['currency'], expiryDate=l['expiryDate'])
                                                                         for l in d.LETTERS_OF_CREDIT if l['customerId'] == cid]) for cid in {l['customerId'] for l in d.LETTERS_OF_CREDIT}}, 'customer',
               ('^CUST-[0-9]{6}$', dict(customerId=qv('customerId'), lettersOfCredit=[]))),
       params=[q('customerId', ID['customer'])], docs={'direction': 'IMPORT (customer is applicant) or EXPORT (customer is beneficiary).'},
       status_docs='ISSUED, AMENDED, DOCUMENTS_PRESENTED, DOCUMENTS_PRESENTED_WITH_DISCREPANCIES, SETTLED or EXPIRED.'),
    op('get_letter_of_credit', 'GET', '/trade/v1/letters-of-credit/details',
       'Get full letter of credit terms: parties and banks, amount, dates, Incoterms, ports, goods, required documents, discrepancies in presented documents, utilisation and settlement UETR.',
       lookup('lcReference', {l['lcReference']: l for l in d.LETTERS_OF_CREDIT}, 'letter of credit'),
       params=[q('lcReference', S('Letter of credit reference, format LC-YYYY-NNNN, e.g. LC-2026-0412.', pattern=r'^LC-[0-9]{4}-[0-9]{4}$'))],
       docs={'direction': 'IMPORT (customer is applicant) or EXPORT (customer is beneficiary).'},
       status_docs='ISSUED, AMENDED, DOCUMENTS_PRESENTED, DOCUMENTS_PRESENTED_WITH_DISCREPANCIES, SETTLED or EXPIRED.'),
    op('list_bank_guarantees', 'GET', '/trade/v1/guarantees',
       'List bank guarantees issued on behalf of a customer (performance bonds, rent and customs guarantees) with beneficiary, amount, expiry and claims.',
       tlookup('customerId', {cid: dict(customerId=cid, guarantees=[{k: v for k, v in g.items() if k != 'customerId'} for g in d.BANK_GUARANTEES if g['customerId'] == cid])
                              for cid in {g['customerId'] for g in d.BANK_GUARANTEES}}, 'customer', ('^CUST-[0-9]{6}$', dict(customerId=qv('customerId'), guarantees=[]))),
       params=[q('customerId', ID['customer'])], status_docs='ACTIVE, CLAIMED, EXPIRED or CANCELLED.'),
])

# ======================================================= 16. wealth portfolio
def portfolio_summary(p):
    return with_staff({k: v for k, v in p.items() if k not in ('holdings', 'customerId')}, 'manager')


upstream('wealth-portfolio', 'Wealth & Investment Portfolios', 'wealth-management',
         'Investment portfolio valuation, holdings and investor suitability (risk profile) for advisory and discretionary clients.', [
    op('get_portfolio_summary', 'GET', '/wealth/v1/portfolios',
       'Get a customer\'s investment portfolios with market value, returns, risk level, mandate type, manager and asset allocation. Returns an empty list for customers without investments.',
       tlookup('customerId', {cid: dict(customerId=cid, totalValue=round(sum(p['marketValue'] for p in d.PORTFOLIOS if p['customerId'] == cid), 2),
                                        portfolios=[portfolio_summary(p) for p in d.PORTFOLIOS if p['customerId'] == cid]) for cid in {p['customerId'] for p in d.PORTFOLIOS}}, 'customer',
               ('^CUST-[0-9]{6}$', dict(customerId=qv('customerId'), totalValue=0.0, portfolios=[]))),
       params=[q('customerId', ID['customer'])]),
    op('list_portfolio_holdings', 'GET', '/wealth/v1/portfolios/holdings',
       'List holdings in a portfolio with instrument, asset class, weight and market value.',
       lookup('portfolioId', {p['portfolioId']: dict(portfolioId=p['portfolioId'], currency=p['currency'], marketValue=p['marketValue'], asOf=d.AS_OF,
                                                     holdings=d.portfolio_holdings(p['portfolioId'])) for p in d.PORTFOLIOS}, 'portfolio'),
       params=[q('portfolioId', S('Portfolio identifier, format PF-NNNNNN.', pattern=r'^PF-[0-9]{6}$'))]),
    op('get_investor_risk_profile', 'GET', '/wealth/v1/risk-profiles',
       'Get the investor suitability assessment: risk profile and level (1-7), capacity for loss, knowledge and experience, horizon, objectives, restrictions and review dates. '
       'Customers without an assessment return profile NOT_ASSESSED.',
       tlookup('customerId', {cid: dict(customerId=cid, **v) for cid, v in d.RISK_PROFILES.items()}, 'customer',
               ('^CUST-[0-9]{6}$', dict(customerId=qv('customerId'), profile='NOT_ASSESSED', riskLevel=None, scale='1 (cautious) to 7 (speculative)', assessedOn=None, nextReviewDue=None,
                                        capacityForLoss=None, knowledgeAndExperience=None, investmentHorizonYears=None, objectives=[], restrictions=[]))),
       params=[q('customerId', ID['customer'])], docs={'restrictions': 'Ethical or other investment restrictions.'}),
])

# ======================================================== 17. product catalog
def product_summary(p):
    return dict(productCode=p['productCode'], name=p['name'], category=p['category'], interestRate=p['interestRate'], rateType=p['rateType'], monthlyFee=p['monthlyFee'])


_categories = ['CURRENT_ACCOUNT', 'SAVINGS', 'CARD', 'PERSONAL_LOAN', 'MORTGAGE', 'BUSINESS', 'CORPORATE']
upstream('product-catalog', 'Product & Pricing Catalogue', 'product-pricing',
         'Published retail, business and corporate products with rates, fees, eligibility and key features.', [
    op('list_banking_products', 'GET', '/products/v1/products',
       'List products, optionally filtered by category, with headline interest rate, rate type and monthly fee. Use get_product_terms for full terms.',
       [rule(c.lower().replace('_', '-'), ok(dict(category=c, products=[product_summary(p) for p in d.PRODUCTS if p['category'] == c])), query={'category': eq(c)}) for c in _categories] +
       [rule('all', ok(dict(category=None, products=[product_summary(p) for p in d.PRODUCTS])), priority=50)],
       params=[q('category', S('Product category filter; omit to list all products.', enum=_categories), required=False)],
       docs={'category': 'Product category.', 'products': 'Products in the catalogue.'}),
    op('get_product_terms', 'GET', '/products/v1/products/terms',
       'Get full terms of a product: interest rate and type, monthly fee, overdraft APR, eligibility and key features.',
       lookup('productCode', {p['productCode']: p for p in d.PRODUCTS}, 'product'), params=[q('productCode', ID['product'])],
       docs={'category': 'Product category.'}),
    op('get_savings_rates', 'GET', '/products/v1/savings-rates',
       'Get current savings rates (AER) for all savings products, with rate type and key restrictions.',
       [rule('current', ok(dict(asOf=d.AS_OF, savingsProducts=[dict(productCode=p['productCode'], name=p['name'], aer=p['interestRate'], rateType=p['rateType'], eligibility=p['eligibility'])
                                                               for p in d.PRODUCTS if p['category'] == 'SAVINGS'])))]),
])

# ===================================================== 18. branch & ATM locator
_cities = sorted({b['city'] for b in d.BRANCHES})
upstream('branch-atm-locator', 'Branch & ATM Network', 'branch-network',
         'Branch and ATM locations with services, opening hours and accessibility, plus branch and video appointment booking.', [
    op('find_branches', 'GET', '/channels/v1/branches',
       'Find branches in a city with address, opening hours, services (mortgage advice, business banking, private banking, trade finance) and step-free access.',
       [trule('city-' + c.lower(), dict(city=qv('city'), branches=[{k: v for k, v in b.items() if k != 'city'} for b in d.BRANCHES if b['city'] == c]), query={'city': rx('(?i)^\\s*' + c + '\\s*$')})
        for c in _cities] + [trule('none', dict(city=qv('city'), branches=[]), priority=500)],
       params=[q('city', S('City name, e.g. London, Manchester, Birmingham, Edinburgh or Bristol.', pattern=r'^[A-Za-z .-]{2,60}$'))]),
    op('find_atms', 'GET', '/channels/v1/atms',
       'Find ATMs in a city with location, postcode, 24-hour availability, deposit capability, accessibility and live service status.',
       [trule('city-' + c.lower(), dict(city=qv('city'), atms=[{k: v for k, v in a.items() if k != 'city'} for a in d.ATMS if a['city'] == c]), query={'city': rx('(?i)^\\s*' + c + '\\s*$')})
        for c in _cities] + [trule('none', dict(city=qv('city'), atms=[]), priority=500)],
       params=[q('city', S('City name, e.g. London.', pattern=r'^[A-Za-z .-]{2,60}$'))], status_docs='IN_SERVICE or OUT_OF_SERVICE.'),
    op('book_branch_appointment', 'POST', '/channels/v1/appointments',
       'Book a branch or video appointment with a specialist. Returns a confirmed appointment ID; the customer receives a confirmation message.',
       [trule('confirmed', dict(appointmentId='APT-' + rnd(6), status='CONFIRMED', customerId=jb('customerId'), branchId=jb('branchId'), topic=jb('topic'), date=jb('preferredDate'),
                                timeSlot=jb('timeSlot'), advisor='A specialist advisor will be allocated 24 hours before the appointment.'), status=201)],
       body=obj(dict(customerId=ID['customer'], branchId=S('Branch identifier from find_branches.', enum=d.BRANCH_IDS),
                     topic=S('Appointment topic.', enum=['MORTGAGE', 'SAVINGS_AND_INVESTMENTS', 'BUSINESS_BANKING', 'PRIVATE_BANKING', 'ACCOUNT_OPENING', 'MONEY_GUIDANCE', 'OTHER']),
                     preferredDate=S('Preferred date (YYYY-MM-DD), a working day in the future.', pattern=DATE),
                     timeSlot=S('Preferred time slot.', enum=['MORNING', 'AFTERNOON', 'VIDEO_CALL'])),
                ['customerId', 'branchId', 'topic', 'preferredDate', 'timeSlot'], 'Appointment request.'), status_docs='CONFIRMED.'),
])

# ================================================= 19. customer notifications
_templates = [t['templateId'] for t in d.NOTIFICATION_TEMPLATES]
_msg = dict(messageId='MSG-' + rnd(8), customerId=jb('customerId'), templateId=jb('templateId'), channel=jb('channel'), sentAt=NOW)
upstream('customer-notifications', 'Customer Notifications', 'digital-engagement',
         'Consent-aware outbound customer messaging over SMS, email, push and secure message, with templates and delivery history.', [
    op('send_customer_notification', 'POST', '/notify/v1/messages',
       'Send a templated message to a customer. The service enforces channel consent: sending on a channel the customer has not consented to returns status REJECTED. '
       'Supply every parameter the template requires (see list_notification_templates). Messages cannot be recalled.',
       [trule('no-sms-consent', dict(_msg, status='REJECTED', reason='NO_CHANNEL_CONSENT', message='Customer has not consented to SMS. Use EMAIL or PUSH.'), status=201, priority=5,
              match=[body_eq('customerId', 'CUST-100106'), body_eq('channel', 'SMS')]),
        trule('no-sms-consent-private', dict(_msg, status='REJECTED', reason='NO_CHANNEL_CONSENT', message='Customer has not consented to SMS. Use SECURE_MESSAGE or EMAIL.'), status=201, priority=5,
              match=[body_rx('customerId', '^(CUST-100103|CUST-300301)$'), body_eq('channel', 'SMS')]),
        trule('queued', dict(_msg, status='QUEUED', reason=None, message='Message queued for delivery.'), status=201, priority=50)],
       body=obj(dict(customerId=ID['customer'], channel=S('Delivery channel; must be consented by the customer.', enum=['SMS', 'EMAIL', 'PUSH', 'SECURE_MESSAGE', 'POST']),
                     templateId=S('Message template.', enum=_templates),
                     parameters=dict(type='object', description='Template parameters as string values, e.g. {"cardLast4": "5546"}.', additionalProperties=dict(type='string', maxLength=200), maxProperties=10)),
                ['customerId', 'channel', 'templateId'], 'Notification request.'), status_docs='QUEUED or REJECTED.'),
    op('list_notification_history', 'GET', '/notify/v1/messages/history',
       'List messages recently sent to a customer with template, channel, delivery status and any customer response.',
       tlookup('customerId', {cid: dict(customerId=cid, notifications=rows) for cid, rows in d.NOTIFICATIONS.items()}, 'customer',
               ('^CUST-[0-9]{6}$', dict(customerId=qv('customerId'), notifications=[]))),
       params=[q('customerId', ID['customer'])], status_docs='QUEUED, DELIVERED, OPENED or FAILED.'),
    op('list_notification_templates', 'GET', '/notify/v1/templates',
       'List available message templates with supported channels, purpose and required parameters.',
       [rule('all', ok(dict(templates=d.NOTIFICATION_TEMPLATES)))]),
])

# =================================================== 20. complaints servicing
upstream('complaints-servicing', 'Complaints Management', 'customer-care',
         'Complaint case management with regulatory deadlines: logging, history, investigation status, outcomes and redress.', [
    op('list_customer_complaints', 'GET', '/complaints/v1/complaints',
       'List a customer\'s complaints with category, status, opening date and final response deadline.',
       tlookup('customerId', {cid: dict(customerId=cid, complaints=[dict(complaintId=c['complaintId'], category=c['category'], status=c['status'], openedOn=c['openedOn'],
                                                                         finalResponseDue=c['finalResponseDue'], summary=c['summary']) for c in d.COMPLAINTS if c['customerId'] == cid])
                              for cid in {c['customerId'] for c in d.COMPLAINTS}}, 'customer', ('^CUST-[0-9]{6}$', dict(customerId=qv('customerId'), complaints=[]))),
       params=[q('customerId', ID['customer'])], status_docs='ACKNOWLEDGED, OPEN, INVESTIGATING, RESOLVED or REFERRED_TO_OMBUDSMAN.'),
    op('get_complaint', 'GET', '/complaints/v1/complaints/details',
       'Get a complaint: category, channel, summary, related reference, handler, acknowledgement date, final response deadline, outcome and redress paid.',
       tlookup('complaintId', {c['complaintId']: with_staff(c, 'handler') for c in d.COMPLAINTS}, 'complaint',
               ('^CMP-[0-9]{6}$', dict(complaintId=qv('complaintId'), customerId=None, category=None, channel=None, status='ACKNOWLEDGED', openedOn=None, summary=None,
                                       relatedReference=None, handler=None, acknowledgedOn=None, finalResponseDue=None, outcome=None, redressPaid=0.0))),
       params=[q('complaintId', S('Complaint identifier, format CMP-NNNNNN.', pattern=r'^CMP-[0-9]{6}$'))],
       status_docs='ACKNOWLEDGED, OPEN, INVESTIGATING, RESOLVED or REFERRED_TO_OMBUDSMAN.'),
    op('log_customer_complaint', 'POST', '/complaints/v1/complaints',
       'Log a new complaint. Any expression of dissatisfaction should be logged. Returns the complaint ID, acknowledgement and the final response deadline (8 weeks).',
       [trule('logged', dict(complaintId='CMP-2' + rnd(5), customerId=jb('customerId'), category=jb('category'), status='ACKNOWLEDGED', acknowledgedOn=days(0),
                             finalResponseDue=days(56), eligibleForOmbudsman='If not resolved by the final response date, the customer may refer the complaint to the Financial Ombudsman Service.'),
              status=201)],
       body=obj(dict(customerId=ID['customer'], category=S('Complaint category.', enum=['CARD_DISPUTE', 'FEES_AND_CHARGES', 'PAYMENT_DELAY', 'SERVICE_QUALITY', 'FRAUD_HANDLING',
                                                                                        'LENDING_DECISION', 'ACCOUNT_ACCESS', 'OTHER']),
                     summary=S('Complaint in the customer\'s words (10-500 characters).', pattern=FREE_TEXT),
                     channel=S('Channel the complaint was received on.', enum=['PHONE', 'BRANCH', 'CHAT', 'EMAIL', 'LETTER', 'RELATIONSHIP_MANAGER']),
                     relatedReference=S('Related record such as a dispute, payment or account ID.', pattern=r'^[A-Za-z0-9-]{3,40}$')),
                ['customerId', 'category', 'summary', 'channel'], 'Complaint details.'), status_docs='ACKNOWLEDGED.'),
])

TOOL_UPSTREAM = {o['name']: u['slug'] for u in UPSTREAMS for o in u['operations']}
UPSTREAM = {u['slug']: u for u in UPSTREAMS}
